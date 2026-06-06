package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	meshcore "github.com/meshcore-dev/meshcore-go"
	"github.com/meshcore-dev/meshcore-go/protocol"
)

// Server owns one long-lived MeshCore client and exposes it over a local Unix
// socket.
type Server struct {
	uri      string
	socket   string
	client   *meshcore.Client
	listener net.Listener

	stopOnce sync.Once
	stopped  chan struct{}
}

// NewServer dials uri and prepares a local backend server.
func NewServer(ctx context.Context, uri string, opts ...meshcore.DialOption) (*Server, error) {
	client, err := meshcore.Dial(ctx, uri, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", uri, err)
	}
	return &Server{
		uri:     uri,
		socket:  SocketPath(),
		client:  client,
		stopped: make(chan struct{}),
	}, nil
}

// Serve listens until Stop is called or the listener fails.
func (s *Server) Serve() error {
	if err := os.MkdirAll(filepath.Dir(s.socket), 0o700); err != nil {
		return err
	}
	if err := os.Remove(s.socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	ln, err := net.Listen("unix", s.socket)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.socket, 0o600); err != nil {
		ln.Close()
		return err
	}
	s.listener = ln
	defer func() {
		ln.Close()
		os.Remove(s.socket)
		s.client.Close()
		close(s.stopped)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopped:
				return nil
			default:
				if errors.Is(err, net.ErrClosed) {
					return nil
				}
				return err
			}
		}
		go s.handle(conn)
	}
}

// Stop closes the listener and the MeshCore client.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		if s.listener != nil {
			s.listener.Close()
		}
	})
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	var req request
	enc := json.NewEncoder(conn)
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = enc.Encode(response{OK: false, Error: err.Error()})
		return
	}
	if req.Method == "watch" {
		s.watch(conn, req.ID)
		return
	}

	result, err := s.dispatch(connContext(), req.Method, req.Params)
	resp := response{ID: req.ID, OK: err == nil}
	if err != nil {
		resp.Error = err.Error()
	} else if result != nil {
		resp.Result, err = json.Marshal(result)
		if err != nil {
			resp.OK = false
			resp.Error = err.Error()
		}
	}
	_ = enc.Encode(resp)

	if req.Method == "stop" && err == nil {
		go s.Stop()
	}
}

func (s *Server) watch(conn net.Conn, id uint64) {
	enc := json.NewEncoder(conn)
	if err := enc.Encode(response{ID: id, OK: true}); err != nil {
		return
	}
	for ev := range s.client.Events() {
		out, ok := backendEvent(ev)
		if !ok {
			continue
		}
		if err := enc.Encode(out); err != nil {
			return
		}
	}
}

func (s *Server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "status":
		return statusResult{Running: true, URI: s.uri, Transport: s.client.Transport(), PID: os.Getpid()}, nil
	case "stop":
		return map[string]bool{"stopping": true}, nil
	case "device_info":
		return s.client.DeviceInfo(ctx)
	case "contacts":
		return s.client.Contacts(ctx)
	case "contact":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return s.client.Contact(ctx, p.Query)
	case "inbox":
		return s.client.SyncMessages(ctx)
	case "send_text":
		var p sendTextParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return s.client.SendText(ctx, p.Recipient, p.Text)
	case "wait_ack":
		var receipt meshcore.Receipt
		if err := json.Unmarshal(params, &receipt); err != nil {
			return nil, err
		}
		return s.client.WaitForAcknowledgement(ctx, receipt)
	case "trace":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return s.client.Trace(ctx, p.Query)
	case "channels":
		return s.client.Channels(ctx)
	case "channel":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return s.client.Channel(ctx, p.Query)
	case "send_channel_text":
		var p channelSendParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return s.client.SendChannelText(ctx, p.Channel, p.Text)
	case "raw_send":
		var p rawParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		msg, err := s.client.RawSend(ctx, p.Payload)
		if err != nil {
			return nil, err
		}
		return RawResultFromMessage(msg), nil
	default:
		return nil, fmt.Errorf("unknown backend method %q", method)
	}
}

func connContext() context.Context {
	return context.Background()
}

func backendEvent(ev meshcore.Event) (Event, bool) {
	switch m := ev.(type) {
	case meshcore.MessageReceived:
		from := m.From.Name
		if m.Channel != "" {
			from = "#" + m.Channel
		}
		return Event{Type: "message", From: from, Channel: m.Channel, Text: m.Text, Timestamp: m.Timestamp}, true
	case meshcore.MessageAcknowledged:
		return Event{Type: "ack", Code: m.Code, RTTMillis: m.RTT.Milliseconds()}, true
	case meshcore.AdvertisementReceived:
		return Event{Type: "advert", Name: m.Contact.Name, PublicKey: m.Contact.PublicKey}, true
	case meshcore.Disconnected:
		msg := ""
		if m.Err != nil {
			msg = m.Err.Error()
		}
		return Event{Type: "disconnected", Error: msg}, true
	default:
		return Event{}, false
	}
}

// RawResultFromMessage converts a decoded SDK protocol message into the stable
// representation used by the backend IPC API.
func RawResultFromMessage(msg protocol.Message) RawResult {
	switch m := msg.(type) {
	case protocol.RawMessage:
		return RawResult{
			Type:    "raw",
			Code:    m.Type,
			Push:    m.Push,
			Payload: m.Payload,
		}
	default:
		return RawResult{
			Type:    fmt.Sprintf("%T", msg),
			Decoded: fmt.Sprintf("%+v", msg),
		}
	}
}
