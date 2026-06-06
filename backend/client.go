package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	meshcore "github.com/meshcore-dev/meshcore-go"
)

const dialTimeout = 250 * time.Millisecond

// Status describes a running backend process.
type Status struct {
	Running   bool
	Healthy   bool
	State     string
	URI       string
	Transport string
	PID       int
	Socket    string
	LastSeen  time.Time
	LastError string
}

// Client talks to a running local backend process.
type Client struct {
	socket string
	nextID atomic.Uint64
}

// NewClient returns a client for socket. If socket is empty, SocketPath is used.
func NewClient(socket string) *Client {
	if socket == "" {
		socket = SocketPath()
	}
	return &Client{socket: socket}
}

// Available reports whether a backend is listening and responding.
func Available(ctx context.Context) bool {
	_, err := NewClient("").Status(ctx)
	return err == nil
}

func (c *Client) Socket() string { return c.socket }

func (c *Client) Status(ctx context.Context) (Status, error) {
	var res statusResult
	if err := c.call(ctx, "status", nil, &res); err != nil {
		return Status{Socket: c.socket}, err
	}
	return Status{
		Running:   res.Running,
		Healthy:   res.Healthy,
		State:     res.State,
		URI:       res.URI,
		Transport: res.Transport,
		PID:       res.PID,
		Socket:    c.socket,
		LastSeen:  res.LastSeen,
		LastError: res.LastError,
	}, nil
}

func (c *Client) Stop(ctx context.Context) error {
	return c.call(ctx, "stop", nil, nil)
}

func (c *Client) DeviceInfo(ctx context.Context) (meshcore.DeviceInfo, error) {
	var out meshcore.DeviceInfo
	err := c.call(ctx, "device_info", nil, &out)
	return out, err
}

func (c *Client) Contacts(ctx context.Context) ([]meshcore.Contact, error) {
	var out []meshcore.Contact
	err := c.call(ctx, "contacts", nil, &out)
	return out, err
}

func (c *Client) Contact(ctx context.Context, name string) (meshcore.Contact, error) {
	var out meshcore.Contact
	err := c.call(ctx, "contact", queryParams{Query: name}, &out)
	return out, err
}

func (c *Client) Inbox(ctx context.Context) ([]meshcore.Message, error) {
	var out []meshcore.Message
	err := c.call(ctx, "inbox", nil, &out)
	return out, err
}

func (c *Client) SendText(ctx context.Context, recipient, text string) (meshcore.Receipt, error) {
	var out meshcore.Receipt
	err := c.call(ctx, "send_text", sendTextParams{Recipient: recipient, Text: text}, &out)
	return out, err
}

func (c *Client) WaitForAcknowledgement(ctx context.Context, receipt meshcore.Receipt) (meshcore.Ack, error) {
	var out meshcore.Ack
	err := c.call(ctx, "wait_ack", receipt, &out)
	return out, err
}

func (c *Client) Trace(ctx context.Context, target string) (meshcore.Trace, error) {
	var out meshcore.Trace
	err := c.call(ctx, "trace", queryParams{Query: target}, &out)
	return out, err
}

func (c *Client) Channels(ctx context.Context) ([]meshcore.Channel, error) {
	var out []meshcore.Channel
	err := c.call(ctx, "channels", nil, &out)
	return out, err
}

func (c *Client) Channel(ctx context.Context, name string) (meshcore.Channel, error) {
	var out meshcore.Channel
	err := c.call(ctx, "channel", queryParams{Query: name}, &out)
	return out, err
}

func (c *Client) SendChannelText(ctx context.Context, channel, text string) (meshcore.Receipt, error) {
	var out meshcore.Receipt
	err := c.call(ctx, "send_channel_text", channelSendParams{Channel: channel, Text: text}, &out)
	return out, err
}

func (c *Client) RawSend(ctx context.Context, payload []byte) (RawResult, error) {
	var out RawResult
	err := c.call(ctx, "raw_send", rawParams{Payload: payload}, &out)
	return out, err
}

func (c *Client) RepeaterLogin(ctx context.Context, repeater, password string) error {
	return c.call(ctx, "repeater_login", repeaterLoginParams{Repeater: repeater, Password: password}, nil)
}

func (c *Client) RepeaterStatus(ctx context.Context, repeater string) (meshcore.RepeaterResponse, error) {
	var out meshcore.RepeaterResponse
	err := c.call(ctx, "repeater_status", queryParams{Query: repeater}, &out)
	return out, err
}

func (c *Client) RepeaterNeighbours(ctx context.Context, repeater string) (meshcore.RepeaterResponse, error) {
	var out meshcore.RepeaterResponse
	err := c.call(ctx, "repeater_neighbours", queryParams{Query: repeater}, &out)
	return out, err
}

func (c *Client) RepeaterExec(ctx context.Context, repeater, command string) (meshcore.RepeaterResponse, error) {
	var out meshcore.RepeaterResponse
	err := c.call(ctx, "repeater_exec", repeaterExecParams{Repeater: repeater, Command: command}, &out)
	return out, err
}

// Watch streams backend events until ctx is cancelled or the backend closes the
// stream.
func (c *Client) Watch(ctx context.Context) (<-chan Event, error) {
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return nil, err
	}

	req := request{ID: c.nextID.Add(1), Method: "watch"}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		conn.Close()
		return nil, err
	}

	dec := json.NewDecoder(bufio.NewReader(conn))
	var resp response
	if err := dec.Decode(&resp); err != nil {
		conn.Close()
		return nil, err
	}
	if !resp.OK {
		conn.Close()
		if resp.Error == "" {
			resp.Error = "unknown backend error"
		}
		return nil, fmt.Errorf("%s", resp.Error)
	}

	out := make(chan Event)
	go func() {
		defer conn.Close()
		defer close(out)
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		for {
			var ev Event
			if err := dec.Decode(&ev); err != nil {
				return
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// WatchRaw streams inbound raw packets until ctx is cancelled or the backend
// closes the stream.
func (c *Client) WatchRaw(ctx context.Context) (<-chan meshcore.RawPacket, error) {
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return nil, err
	}

	req := request{ID: c.nextID.Add(1), Method: "watch_raw"}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		conn.Close()
		return nil, err
	}

	dec := json.NewDecoder(bufio.NewReader(conn))
	var resp response
	if err := dec.Decode(&resp); err != nil {
		conn.Close()
		return nil, err
	}
	if !resp.OK {
		conn.Close()
		if resp.Error == "" {
			resp.Error = "unknown backend error"
		}
		return nil, fmt.Errorf("%s", resp.Error)
	}

	out := make(chan meshcore.RawPacket)
	go func() {
		defer conn.Close()
		defer close(out)
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		for {
			var pkt meshcore.RawPacket
			if err := dec.Decode(&pkt); err != nil {
				return
			}
			select {
			case out <- pkt:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) call(ctx context.Context, method string, params, out any) error {
	d := net.Dialer{Timeout: dialTimeout}

	conn, err := d.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return err
	}
	defer conn.Close()

	req := request{ID: c.nextID.Add(1), Method: method}
	if params != nil {
		req.Params, err = json.Marshal(params)
		if err != nil {
			return err
		}
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}

	var resp response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return err
	}
	if resp.ID != req.ID {
		return fmt.Errorf("backend: mismatched response id %d", resp.ID)
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "unknown backend error"
		}
		return fmt.Errorf("%s", resp.Error)
	}
	if out == nil || len(resp.Result) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Result, out)
}
