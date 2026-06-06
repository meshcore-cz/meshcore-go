// Package service contains command semantics shared by CLI frontends and the
// future local daemon.
package service

import (
	"context"

	meshcore "github.com/meshcore-dev/meshcore-go"
)

// Service exposes high-level operations over a connected MeshCore client.
type Service struct {
	client *meshcore.Client
}

// New returns a service bound to client. The caller owns the client lifetime.
func New(client *meshcore.Client) *Service {
	return &Service{client: client}
}

func (s *Service) DeviceInfo(ctx context.Context) (meshcore.DeviceInfo, error) {
	return s.client.DeviceInfo(ctx)
}

func (s *Service) Contacts(ctx context.Context) ([]meshcore.Contact, error) {
	return s.client.Contacts(ctx)
}

func (s *Service) Contact(ctx context.Context, name string) (meshcore.Contact, error) {
	return s.client.Contact(ctx, name)
}

func (s *Service) Inbox(ctx context.Context) ([]meshcore.Message, error) {
	return s.client.SyncMessages(ctx)
}

func (s *Service) SendText(ctx context.Context, recipient, text string) (meshcore.Receipt, error) {
	return s.client.SendText(ctx, recipient, text)
}

func (s *Service) WaitForAcknowledgement(ctx context.Context, receipt meshcore.Receipt) (meshcore.Ack, error) {
	return s.client.WaitForAcknowledgement(ctx, receipt)
}

func (s *Service) Trace(ctx context.Context, target string) (meshcore.Trace, error) {
	return s.client.Trace(ctx, target)
}

func (s *Service) Channels(ctx context.Context) ([]meshcore.Channel, error) {
	return s.client.Channels(ctx)
}

func (s *Service) Channel(ctx context.Context, name string) (meshcore.Channel, error) {
	return s.client.Channel(ctx, name)
}

func (s *Service) SendChannelText(ctx context.Context, channel, text string) (meshcore.Receipt, error) {
	return s.client.SendChannelText(ctx, channel, text)
}

func (s *Service) Events() <-chan meshcore.Event {
	return s.client.Events()
}
