package cli

import (
	"context"
	"errors"
	"fmt"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/service"
)

// Backend is the command execution surface used by CLI commands. Today the
// direct implementation owns a meshcore.Client; the daemon implementation will
// speak the same surface over local IPC.
type Backend interface {
	URI() string
	Transport() string
	DeviceInfo(context.Context) (meshcore.DeviceInfo, error)
	Stats(context.Context) (meshcore.LocalStats, error)
	Contacts(context.Context) ([]meshcore.Contact, error)
	ContactsWithOptions(context.Context, bool, bool, bool, bool) ([]meshcore.Contact, error)
	StartContactRefresh(context.Context, bool) (localbackend.ContactRefreshResult, error)
	Contact(context.Context, string) (meshcore.Contact, error)
	Inbox(context.Context) ([]meshcore.Message, error)
	SendText(context.Context, string, string) (meshcore.Receipt, error)
	WaitForAcknowledgement(context.Context, meshcore.Receipt) (meshcore.Ack, error)
	Trace(context.Context, string) (meshcore.Trace, error)
	Channels(context.Context) ([]meshcore.Channel, error)
	ChannelsWithOptions(context.Context, bool) ([]meshcore.Channel, error)
	Channel(context.Context, string) (meshcore.Channel, error)
	SendChannelText(context.Context, string, string) (meshcore.Receipt, error)
	ChannelAdd(context.Context, string, []byte) (meshcore.Channel, error)
	ChannelRemove(context.Context, string) (meshcore.Channel, error)
	Advertise(context.Context, bool) error
	DiscoverNodes(context.Context, meshcore.NodeDiscoverOptions, func(meshcore.DiscoveredNode)) ([]meshcore.DiscoveredNode, error)
	RawSend(context.Context, []byte) (localbackend.RawResult, error)
	RepeaterHasConnection(context.Context, string) (bool, error)
	RepeaterLogin(context.Context, string, string) (meshcore.RepeaterSession, error)
	RepeaterStatus(context.Context, string) (meshcore.RepeaterResponse, error)
	RepeaterNeighbours(context.Context, string) (meshcore.RepeaterResponse, error)
	RepeaterExec(context.Context, string, string) (meshcore.RepeaterResponse, error)
	Events() <-chan meshcore.Event
	Close() error
}

var errBackendDegraded = errors.New("backend degraded")

type directBackend struct {
	uri    string
	client *meshcore.Client
	*service.Service
}

func resolveBackendSocket(e *env) string {
	if e.exec.BackendSocket != "" {
		return e.exec.BackendSocket
	}
	return localbackend.SocketPath()
}

func backendClientForEnv(e *env) *localbackend.Client {
	return localbackend.NewClientForDevice(resolveBackendSocket(e), e.args.flag("device"))
}

// daemonRunning reports whether the backend daemon (supervisor) is up,
// independent of whether any device session is connected. This is distinct from
// backendStatus, which reports the targeted/default device session's health.
func daemonRunning(ctx context.Context, e *env) (localbackend.DaemonStatus, bool) {
	socket := resolveBackendSocket(e)
	c := localbackend.NewClient(socket)
	if st, err := c.BackendStatus(ctx); err == nil {
		return st, true
	}
	// Legacy single-session daemon without backend_status: fall back to the
	// per-device status method to detect liveness.
	if _, err := c.Status(ctx); err == nil {
		return localbackend.DaemonStatus{Running: true, Socket: socket}, true
	}
	return localbackend.DaemonStatus{Socket: socket}, false
}

func openBackend(ctx context.Context, e *env) (Backend, error) {
	if preferIPCBackend(e) {
		b, err := openIPCBackend(ctx, e)
		if err == nil {
			e.dbg.Backend("ipc", b)
			return b, nil
		}
		if errors.Is(err, errBackendDegraded) {
			return nil, err
		}
		if e.exec.RequireIPC {
			return nil, fmt.Errorf("shell backend unavailable: %w", err)
		}
		e.dbg.Log("ipc backend unavailable", "error", err)
	}
	b, err := openDirectBackend(ctx, e)
	if err == nil {
		e.dbg.Backend("direct", b)
	}
	return b, err
}

// preferIPCBackend reports whether a command should use the local daemon when
// available. --device routes to the matching daemon session; only an explicit
// --uri (a temporary endpoint) or --direct bypasses the daemon.
func preferIPCBackend(e *env) bool {
	return !e.args.has("direct") && e.args.flag("uri") == ""
}

func openIPCBackend(ctx context.Context, e *env) (*ipcBackend, error) {
	client := backendClientForEnv(e)
	status, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Healthy {
		msg := fmt.Sprintf("backend is %s", status.State)
		if status.LastError != "" {
			msg += ": " + status.LastError
		}
		return nil, fmt.Errorf("%w: %s", errBackendDegraded, msg)
	}
	return &ipcBackend{Client: client, status: status}, nil
}

func openIPCBackendAllowDegraded(ctx context.Context, e *env) (*ipcBackend, error) {
	client := backendClientForEnv(e)
	status, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	return &ipcBackend{Client: client, status: status}, nil
}

func openDirectBackend(ctx context.Context, e *env) (*directBackend, error) {
	client, uri, err := connect(ctx, e)
	if err != nil {
		return nil, err
	}
	return newDirectBackend(uri, client), nil
}

func newDirectBackend(uri string, client *meshcore.Client) *directBackend {
	return &directBackend{
		uri:     uri,
		client:  client,
		Service: service.New(client),
	}
}

func (b *directBackend) URI() string { return b.uri }

func (b *directBackend) Transport() string { return b.client.Transport() }

func (b *directBackend) ContactsWithOptions(ctx context.Context, cached, refresh, wait, full bool) ([]meshcore.Contact, error) {
	if cached {
		return nil, fmt.Errorf("contact local state requires the backend")
	}
	if refresh {
		return nil, fmt.Errorf("contact refresh requires the backend")
	}
	return b.Contacts(ctx)
}

func (b *directBackend) StartContactRefresh(ctx context.Context, full bool) (localbackend.ContactRefreshResult, error) {
	return localbackend.ContactRefreshResult{}, fmt.Errorf("contact refresh requires the backend")
}

func (b *directBackend) ChannelsWithOptions(ctx context.Context, refresh bool) ([]meshcore.Channel, error) {
	return b.Channels(ctx)
}

func (b *directBackend) ChannelAdd(ctx context.Context, name string, secret []byte) (meshcore.Channel, error) {
	return b.client.AddChannel(ctx, name, secret)
}

func (b *directBackend) ChannelRemove(ctx context.Context, channel string) (meshcore.Channel, error) {
	return b.client.RemoveChannel(ctx, channel)
}

func (b *directBackend) RawSend(ctx context.Context, payload []byte) (localbackend.RawResult, error) {
	msg, err := b.client.RawSend(ctx, payload)
	if err != nil {
		return localbackend.RawResult{}, err
	}
	return localbackend.RawResultFromMessage(msg), nil
}

func (b *directBackend) Events() <-chan meshcore.Event {
	return b.Service.Events()
}

func (b *directBackend) Close() error {
	return b.client.Close()
}

type ipcBackend struct {
	*localbackend.Client
	status localbackend.Status
}

func (b *ipcBackend) URI() string { return b.status.URI }

func (b *ipcBackend) Transport() string { return b.status.Transport }

func (b *ipcBackend) DiscoverNodes(ctx context.Context, opts meshcore.NodeDiscoverOptions, onNode func(meshcore.DiscoveredNode)) ([]meshcore.DiscoveredNode, error) {
	nodes, err := b.Discover(ctx, opts.Filter, opts.PrefixOnly, opts.Timeout)
	if err != nil {
		return nil, err
	}
	var out []meshcore.DiscoveredNode
	for n := range nodes {
		out = append(out, n)
		if onNode != nil {
			onNode(n)
		}
	}
	return out, nil
}

func (b *ipcBackend) Events() <-chan meshcore.Event {
	return nil
}

func (b *ipcBackend) Close() error {
	return nil
}
