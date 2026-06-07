package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

const dialTimeout = 250 * time.Millisecond

// DeviceStatus is a snapshot of the connected radio served without blocking on
// active radio operations.
type DeviceStatus struct {
	Name            string
	PublicKey       string
	Firmware        string
	FirmwareVersion string
	Protocol        string
	Transport       string
	Capabilities    []string
	RadioFreqKHz    uint32
	RadioBWKHz      uint32
	RadioSF         byte
	RadioCR         byte
	TxPowerDBm      byte
}

func (d DeviceStatus) Available() bool {
	return d.Name != "" || d.PublicKey != ""
}

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
	Bridges   []BridgeStatus
	Contacts  ContactStatus
	Channels  ChannelStatus
	Device    DeviceStatus
	Stats     meshcore.LocalStats
	StatsOK   bool
	StatsAt   time.Time
	Radio     RadioStatus
}

// ContactStatus describes the backend's local contact replica.
type ContactStatus struct {
	Syncing      bool
	SyncReceived int
	SyncTotal    int
	Count        int
	SyncedAt     time.Time
	Error        string
}

// ChannelStatus describes the backend's local channel replica.
type ChannelStatus struct {
	Syncing  bool
	Count    int
	SyncedAt time.Time
	Error    string
}

// RadioStatus describes whether the backend transport is busy with radio I/O.
type RadioStatus struct {
	Active     bool
	Idle       bool
	Method     string
	Since      time.Time
	DurationMs     int64
	LastAt         time.Time
	LastMethod     string
	LastDurationMs int64
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
	st := Status{
		Running:   res.Running,
		Healthy:   res.Healthy,
		State:     res.State,
		URI:       res.URI,
		Transport: res.Transport,
		PID:       res.PID,
		Socket:    c.socket,
		LastSeen:  res.LastSeen,
		LastError: res.LastError,
		Bridges:   res.Bridges,
		Contacts: ContactStatus{
			Syncing:      res.Contacts.Syncing,
			SyncReceived: res.Contacts.SyncReceived,
			SyncTotal:    res.Contacts.SyncTotal,
			Count:        res.Contacts.Count,
			SyncedAt:     res.Contacts.SyncedAt,
			Error:        res.Contacts.Error,
		},
		Channels: ChannelStatus{
			Syncing:  res.Channels.Syncing,
			Count:    res.Channels.Count,
			SyncedAt: res.Channels.SyncedAt,
			Error:    res.Channels.Error,
		},
		Radio: RadioStatus{
			Active:         res.Radio.Active,
			Idle:           res.Radio.Idle,
			Method:         res.Radio.Method,
			Since:          res.Radio.Since,
			DurationMs:     res.Radio.DurationMs,
			LastAt:         res.Radio.LastAt,
			LastMethod:     res.Radio.LastMethod,
			LastDurationMs: res.Radio.LastDurationMs,
		},
	}
	if res.Device != nil {
		st.Device = DeviceStatus{
			Name:            res.Device.Name,
			PublicKey:       res.Device.PublicKey,
			Firmware:        res.Device.Firmware,
			FirmwareVersion: res.Device.FirmwareVersion,
			Protocol:        res.Device.Protocol,
			Capabilities:    append([]string(nil), res.Device.Capabilities...),
			RadioFreqKHz:    res.Device.RadioFreqKHz,
			RadioBWKHz:      res.Device.RadioBWKHz,
			RadioSF:         res.Device.RadioSF,
			RadioCR:         res.Device.RadioCR,
			TxPowerDBm:      res.Device.TxPowerDBm,
		}
	}
	if res.Stats != nil {
		st.Stats = *res.Stats
		st.StatsOK = true
		st.StatsAt = res.StatsAt
	}
	return st, nil
}

func (c *Client) Stop(ctx context.Context) error {
	return c.call(ctx, "stop", nil, nil)
}

func (c *Client) DeviceInfo(ctx context.Context) (meshcore.DeviceInfo, error) {
	var out meshcore.DeviceInfo
	err := c.call(ctx, "device_info", nil, &out)
	return out, err
}

func (c *Client) Stats(ctx context.Context) (meshcore.LocalStats, error) {
	return c.StatsWithOptions(ctx, false)
}

func (c *Client) StatsWithOptions(ctx context.Context, refresh bool) (meshcore.LocalStats, error) {
	var out meshcore.LocalStats
	err := c.call(ctx, "stats", statsParams{Refresh: refresh}, &out)
	return out, err
}

func (c *Client) Contacts(ctx context.Context) ([]meshcore.Contact, error) {
	return c.ContactsWithOptions(ctx, false, false, false, false)
}

func (c *Client) ContactsWithOptions(ctx context.Context, cached, refresh, wait, full bool) ([]meshcore.Contact, error) {
	var out []meshcore.Contact
	err := c.call(ctx, "contacts", contactsParams{Cached: cached, Refresh: refresh, Wait: wait, Full: full}, &out)
	return out, err
}

func (c *Client) StartContactRefresh(ctx context.Context, full bool) (ContactRefreshResult, error) {
	var out ContactRefreshResult
	err := c.call(ctx, "contacts", contactsParams{Refresh: true, Wait: false, Full: full}, &out)
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
	return c.ChannelsWithOptions(ctx, false)
}

func (c *Client) ChannelsWithOptions(ctx context.Context, refresh bool) ([]meshcore.Channel, error) {
	var out []meshcore.Channel
	err := c.call(ctx, "channels", channelsParams{Refresh: refresh}, &out)
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

func (c *Client) Advertise(ctx context.Context, flood bool) error {
	return c.call(ctx, "advert", advertParams{Flood: flood}, nil)
}

// Discover streams discovered nodes from a node-discovery scan until the
// backend closes the stream (after the discovery window elapses) or ctx is
// cancelled.
func (c *Client) Discover(ctx context.Context, filter byte, prefixOnly bool, timeout time.Duration) (<-chan meshcore.DiscoveredNode, error) {
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return nil, err
	}

	params := discoverParams{Filter: filter, PrefixOnly: prefixOnly, TimeoutMs: int(timeout.Milliseconds())}
	req := request{ID: c.nextID.Add(1), Method: "discover"}
	if req.Params, err = json.Marshal(params); err != nil {
		conn.Close()
		return nil, err
	}
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

	out := make(chan meshcore.DiscoveredNode)
	go func() {
		defer conn.Close()
		defer close(out)
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		for {
			var n meshcore.DiscoveredNode
			if err := dec.Decode(&n); err != nil {
				return
			}
			select {
			case out <- n:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) RawSend(ctx context.Context, payload []byte) (RawResult, error) {
	var out RawResult
	err := c.call(ctx, "raw_send", rawParams{Payload: payload}, &out)
	return out, err
}

func (c *Client) RepeaterHasConnection(ctx context.Context, repeater string) (bool, error) {
	var out repeaterHasConnectionResult
	err := c.call(ctx, "repeater_has_connection", queryParams{Query: repeater}, &out)
	return out.Active, err
}

func (c *Client) RepeaterLogin(ctx context.Context, repeater, password string) (meshcore.RepeaterSession, error) {
	var out meshcore.RepeaterSession
	err := c.call(ctx, "repeater_login", repeaterLoginParams{Repeater: repeater, Password: password}, &out)
	return out, err
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
