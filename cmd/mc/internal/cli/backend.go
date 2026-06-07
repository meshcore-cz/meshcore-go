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
	svc    *service.Service
}

func openBackend(ctx context.Context, e *env) (Backend, error) {
	if preferIPCBackend(e) {
		b, err := openIPCBackend(ctx)
		if err == nil {
			e.dbg.Backend("ipc", b)
			return b, nil
		}
		if errors.Is(err, errBackendDegraded) {
			return nil, err
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
// available. Explicit --uri, --device, or --direct always dial directly.
func preferIPCBackend(e *env) bool {
	return !e.args.has("direct") &&
		e.args.flag("uri") == "" &&
		e.args.flag("device") == ""
}

func openIPCBackend(ctx context.Context) (*ipcBackend, error) {
	client := localbackend.NewClient("")
	status, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Healthy {
		msg := fmt.Sprintf("current active device is %s", status.State)
		if status.LastError != "" {
			msg += ": " + status.LastError
		}
		return nil, fmt.Errorf("%w: %s", errBackendDegraded, msg)
	}
	return &ipcBackend{client: client, status: status}, nil
}

func openIPCBackendAllowDegraded(ctx context.Context) (*ipcBackend, error) {
	client := localbackend.NewClient("")
	status, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	return &ipcBackend{client: client, status: status}, nil
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
		uri:    uri,
		client: client,
		svc:    service.New(client),
	}
}

func (b *directBackend) URI() string { return b.uri }

func (b *directBackend) Transport() string { return b.client.Transport() }

func (b *directBackend) DeviceInfo(ctx context.Context) (meshcore.DeviceInfo, error) {
	return b.svc.DeviceInfo(ctx)
}

func (b *directBackend) Stats(ctx context.Context) (meshcore.LocalStats, error) {
	return b.svc.Stats(ctx)
}

func (b *directBackend) Contacts(ctx context.Context) ([]meshcore.Contact, error) {
	return b.svc.Contacts(ctx)
}

func (b *directBackend) ContactsWithOptions(ctx context.Context, cached, refresh, wait, full bool) ([]meshcore.Contact, error) {
	if cached {
		return nil, fmt.Errorf("local contact replica requires the backend")
	}
	if refresh {
		return nil, fmt.Errorf("contact refresh requires the backend")
	}
	return b.svc.Contacts(ctx)
}

func (b *directBackend) StartContactRefresh(ctx context.Context, full bool) (localbackend.ContactRefreshResult, error) {
	return localbackend.ContactRefreshResult{}, fmt.Errorf("contact refresh requires the backend")
}

func (b *directBackend) Contact(ctx context.Context, name string) (meshcore.Contact, error) {
	return b.svc.Contact(ctx, name)
}

func (b *directBackend) Inbox(ctx context.Context) ([]meshcore.Message, error) {
	return b.svc.Inbox(ctx)
}

func (b *directBackend) SendText(ctx context.Context, recipient, text string) (meshcore.Receipt, error) {
	return b.svc.SendText(ctx, recipient, text)
}

func (b *directBackend) WaitForAcknowledgement(ctx context.Context, receipt meshcore.Receipt) (meshcore.Ack, error) {
	return b.svc.WaitForAcknowledgement(ctx, receipt)
}

func (b *directBackend) Trace(ctx context.Context, target string) (meshcore.Trace, error) {
	return b.svc.Trace(ctx, target)
}

func (b *directBackend) Channels(ctx context.Context) ([]meshcore.Channel, error) {
	return b.svc.Channels(ctx)
}

func (b *directBackend) ChannelsWithOptions(ctx context.Context, refresh bool) ([]meshcore.Channel, error) {
	return b.svc.Channels(ctx)
}

func (b *directBackend) Channel(ctx context.Context, name string) (meshcore.Channel, error) {
	return b.svc.Channel(ctx, name)
}

func (b *directBackend) SendChannelText(ctx context.Context, channel, text string) (meshcore.Receipt, error) {
	return b.svc.SendChannelText(ctx, channel, text)
}

func (b *directBackend) Advertise(ctx context.Context, flood bool) error {
	return b.svc.Advertise(ctx, flood)
}

func (b *directBackend) DiscoverNodes(ctx context.Context, opts meshcore.NodeDiscoverOptions, onNode func(meshcore.DiscoveredNode)) ([]meshcore.DiscoveredNode, error) {
	return b.svc.DiscoverNodes(ctx, opts, onNode)
}

func (b *directBackend) RawSend(ctx context.Context, payload []byte) (localbackend.RawResult, error) {
	msg, err := b.client.RawSend(ctx, payload)
	if err != nil {
		return localbackend.RawResult{}, err
	}
	return localbackend.RawResultFromMessage(msg), nil
}

func (b *directBackend) RepeaterHasConnection(ctx context.Context, repeater string) (bool, error) {
	return b.svc.RepeaterHasConnection(ctx, repeater)
}

func (b *directBackend) RepeaterLogin(ctx context.Context, repeater, password string) (meshcore.RepeaterSession, error) {
	return b.svc.RepeaterLogin(ctx, repeater, password)
}

func (b *directBackend) RepeaterStatus(ctx context.Context, repeater string) (meshcore.RepeaterResponse, error) {
	return b.svc.RepeaterStatus(ctx, repeater)
}

func (b *directBackend) RepeaterNeighbours(ctx context.Context, repeater string) (meshcore.RepeaterResponse, error) {
	return b.svc.RepeaterNeighbours(ctx, repeater)
}

func (b *directBackend) RepeaterExec(ctx context.Context, repeater, command string) (meshcore.RepeaterResponse, error) {
	return b.svc.RepeaterExec(ctx, repeater, command)
}

func (b *directBackend) Events() <-chan meshcore.Event {
	return b.svc.Events()
}

func (b *directBackend) Close() error {
	return b.client.Close()
}

type ipcBackend struct {
	client *localbackend.Client
	status localbackend.Status
}

func (b *ipcBackend) URI() string { return b.status.URI }

func (b *ipcBackend) Transport() string { return b.status.Transport }

func (b *ipcBackend) DeviceInfo(ctx context.Context) (meshcore.DeviceInfo, error) {
	return b.client.DeviceInfo(ctx)
}

func (b *ipcBackend) Stats(ctx context.Context) (meshcore.LocalStats, error) {
	return b.client.Stats(ctx)
}

func (b *ipcBackend) Contacts(ctx context.Context) ([]meshcore.Contact, error) {
	return b.client.Contacts(ctx)
}

func (b *ipcBackend) ContactsWithOptions(ctx context.Context, cached, refresh, wait, full bool) ([]meshcore.Contact, error) {
	return b.client.ContactsWithOptions(ctx, cached, refresh, wait, full)
}

func (b *ipcBackend) StartContactRefresh(ctx context.Context, full bool) (localbackend.ContactRefreshResult, error) {
	return b.client.StartContactRefresh(ctx, full)
}

func (b *ipcBackend) Contact(ctx context.Context, name string) (meshcore.Contact, error) {
	return b.client.Contact(ctx, name)
}

func (b *ipcBackend) Inbox(ctx context.Context) ([]meshcore.Message, error) {
	return b.client.Inbox(ctx)
}

func (b *ipcBackend) SendText(ctx context.Context, recipient, text string) (meshcore.Receipt, error) {
	return b.client.SendText(ctx, recipient, text)
}

func (b *ipcBackend) WaitForAcknowledgement(ctx context.Context, receipt meshcore.Receipt) (meshcore.Ack, error) {
	return b.client.WaitForAcknowledgement(ctx, receipt)
}

func (b *ipcBackend) Trace(ctx context.Context, target string) (meshcore.Trace, error) {
	return b.client.Trace(ctx, target)
}

func (b *ipcBackend) Channels(ctx context.Context) ([]meshcore.Channel, error) {
	return b.client.Channels(ctx)
}

func (b *ipcBackend) ChannelsWithOptions(ctx context.Context, refresh bool) ([]meshcore.Channel, error) {
	return b.client.ChannelsWithOptions(ctx, refresh)
}

func (b *ipcBackend) Channel(ctx context.Context, name string) (meshcore.Channel, error) {
	return b.client.Channel(ctx, name)
}

func (b *ipcBackend) SendChannelText(ctx context.Context, channel, text string) (meshcore.Receipt, error) {
	return b.client.SendChannelText(ctx, channel, text)
}

func (b *ipcBackend) Advertise(ctx context.Context, flood bool) error {
	return b.client.Advertise(ctx, flood)
}

func (b *ipcBackend) DiscoverNodes(ctx context.Context, opts meshcore.NodeDiscoverOptions, onNode func(meshcore.DiscoveredNode)) ([]meshcore.DiscoveredNode, error) {
	nodes, err := b.client.Discover(ctx, opts.Filter, opts.PrefixOnly, opts.Timeout)
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

func (b *ipcBackend) RawSend(ctx context.Context, payload []byte) (localbackend.RawResult, error) {
	return b.client.RawSend(ctx, payload)
}

func (b *ipcBackend) RepeaterHasConnection(ctx context.Context, repeater string) (bool, error) {
	return b.client.RepeaterHasConnection(ctx, repeater)
}

func (b *ipcBackend) RepeaterLogin(ctx context.Context, repeater, password string) (meshcore.RepeaterSession, error) {
	return b.client.RepeaterLogin(ctx, repeater, password)
}

func (b *ipcBackend) RepeaterStatus(ctx context.Context, repeater string) (meshcore.RepeaterResponse, error) {
	return b.client.RepeaterStatus(ctx, repeater)
}

func (b *ipcBackend) RepeaterNeighbours(ctx context.Context, repeater string) (meshcore.RepeaterResponse, error) {
	return b.client.RepeaterNeighbours(ctx, repeater)
}

func (b *ipcBackend) RepeaterExec(ctx context.Context, repeater, command string) (meshcore.RepeaterResponse, error) {
	return b.client.RepeaterExec(ctx, repeater, command)
}

func (b *ipcBackend) Events() <-chan meshcore.Event {
	return nil
}

func (b *ipcBackend) Close() error {
	return nil
}
