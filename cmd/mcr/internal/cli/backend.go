package cli

import (
	"context"

	meshcore "github.com/meshcore-dev/meshcore-go"
	localbackend "github.com/meshcore-dev/meshcore-go/backend"
	"github.com/meshcore-dev/meshcore-go/cmd/mcr/internal/service"
)

// Backend is the command execution surface used by CLI commands. Today the
// direct implementation owns a meshcore.Client; the daemon implementation will
// speak the same surface over local IPC.
type Backend interface {
	URI() string
	Transport() string
	DeviceInfo(context.Context) (meshcore.DeviceInfo, error)
	Contacts(context.Context) ([]meshcore.Contact, error)
	Contact(context.Context, string) (meshcore.Contact, error)
	Inbox(context.Context) ([]meshcore.Message, error)
	SendText(context.Context, string, string) (meshcore.Receipt, error)
	WaitForAcknowledgement(context.Context, meshcore.Receipt) (meshcore.Ack, error)
	Trace(context.Context, string) (meshcore.Trace, error)
	Channels(context.Context) ([]meshcore.Channel, error)
	Channel(context.Context, string) (meshcore.Channel, error)
	SendChannelText(context.Context, string, string) (meshcore.Receipt, error)
	RawSend(context.Context, []byte) (localbackend.RawResult, error)
	Events() <-chan meshcore.Event
	Close() error
}

type directBackend struct {
	uri    string
	client *meshcore.Client
	svc    *service.Service
}

func openBackend(ctx context.Context, e *env) (Backend, error) {
	if !e.args.has("direct") {
		if b, err := openIPCBackend(ctx); err == nil {
			return b, nil
		}
	}
	return openDirectBackend(ctx, e)
}

func openIPCBackend(ctx context.Context) (*ipcBackend, error) {
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

func (b *directBackend) Contacts(ctx context.Context) ([]meshcore.Contact, error) {
	return b.svc.Contacts(ctx)
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

func (b *directBackend) Channel(ctx context.Context, name string) (meshcore.Channel, error) {
	return b.svc.Channel(ctx, name)
}

func (b *directBackend) SendChannelText(ctx context.Context, channel, text string) (meshcore.Receipt, error) {
	return b.svc.SendChannelText(ctx, channel, text)
}

func (b *directBackend) RawSend(ctx context.Context, payload []byte) (localbackend.RawResult, error) {
	msg, err := b.client.RawSend(ctx, payload)
	if err != nil {
		return localbackend.RawResult{}, err
	}
	return localbackend.RawResultFromMessage(msg), nil
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

func (b *ipcBackend) Contacts(ctx context.Context) ([]meshcore.Contact, error) {
	return b.client.Contacts(ctx)
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

func (b *ipcBackend) Channel(ctx context.Context, name string) (meshcore.Channel, error) {
	return b.client.Channel(ctx, name)
}

func (b *ipcBackend) SendChannelText(ctx context.Context, channel, text string) (meshcore.Receipt, error) {
	return b.client.SendChannelText(ctx, channel, text)
}

func (b *ipcBackend) RawSend(ctx context.Context, payload []byte) (localbackend.RawResult, error) {
	return b.client.RawSend(ctx, payload)
}

func (b *ipcBackend) Events() <-chan meshcore.Event {
	return nil
}

func (b *ipcBackend) Close() error {
	return nil
}
