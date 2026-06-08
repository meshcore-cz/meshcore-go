package backend

import (
	"context"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

const inboxDrainTimeout = 30 * time.Second

// drainLoop is the session's single inbox drainer. The backend is the only
// consumer that drains the radio inbox: it runs an initial drain on connect and
// re-drains whenever the device signals MSG_WAITING (observeEvent).
func (s *DeviceSession) drainLoop() {
	s.runDrain() // initial drain on connect
	for {
		select {
		case <-s.stopped:
			return
		case <-s.drainReq:
			s.runDrain()
		}
	}
}

// requestDrain asks the drain loop to drain the inbox (non-blocking, coalesced).
func (s *DeviceSession) requestDrain() {
	select {
	case s.drainReq <- struct{}{}:
	default:
	}
}

// runDrain drains the device inbox, persisting each message to local state
// before the SDK broadcasts its MessageReceived event. Draining is serialised
// with other radio I/O via the radio lock.
func (s *DeviceSession) runDrain() {
	if s.stateSnapshot() == stateBridge {
		return
	}
	s.lockRadio("inbox")
	defer s.unlockRadio()

	client := s.clientSnapshot()
	if client == nil || !s.healthy() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), inboxDrainTimeout)
	defer cancel()

	err := client.DrainMessages(ctx, func(m meshcore.Message) error {
		s.persistIncoming(ctx, m)
		return nil
	})
	if err != nil {
		Logf("inbox drain failed: %v", err)
	}
}

// persistIncoming stores one received message as device-local state.
func (s *DeviceSession) persistIncoming(ctx context.Context, m meshcore.Message) {
	if s.store == nil {
		return
	}
	rec := MessageRecord{
		Direction: MessageIn,
		Kind:      MessageDirect,
		Peer:      m.From,
		Text:      m.Text,
		TxtType:   m.TxtType,
		Timestamp: m.Timestamp,
		SNR:       m.SNR,
		Status:    StatusReceived,
	}
	if m.Channel != "" {
		// Resolve the device slot index to the channel's universal key + name.
		rec.Kind = MessageChannel
		rec.Channel = m.Channel
		rec.Peer = ""
		if entry, err := s.store.Channel(ctx, m.Channel); err == nil {
			rec.Peer = entry.Key
			rec.PeerName = entry.Channel.Name
		}
	} else if entry, err := s.store.Contact(ctx, m.From); err == nil {
		// Resolve the 6-byte sender prefix to the contact's full public key.
		if entry.Contact.PublicKey != "" {
			rec.Peer = entry.Contact.PublicKey
		}
		rec.PeerName = entry.Contact.Name
	}
	hearing := Reception{At: time.Now().UTC(), SNR: m.SNR, PathLen: int(m.PathLen)}
	if _, err := s.store.RecordReceivedMessage(ctx, &rec, hearing); err != nil {
		Logf("persist incoming message failed: %v", err)
	}
}

// persistOutgoing stores an outgoing message before it is sent and returns its
// row id so the caller can update the delivery status.
func (s *DeviceSession) persistOutgoing(ctx context.Context, rec MessageRecord) int64 {
	if s.store == nil {
		return 0
	}
	rec.Direction = MessageOut
	rec.Read = true
	if rec.Status == "" {
		rec.Status = StatusQueued
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}
	if err := s.store.InsertMessage(ctx, &rec); err != nil {
		Logf("persist outgoing message failed: %v", err)
		return 0
	}
	return rec.ID
}

func (s *DeviceSession) setMessageStatus(id int64, status, ackCode string) {
	if s.store == nil || id == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.SetMessageStatus(ctx, id, status, ackCode); err != nil {
		Logf("update message status failed: %v", err)
	}
}

// recordAck appends an acknowledgement (one mesh path's confirmation) to the
// outgoing message(s) with ackCode and marks them delivered. Called for every
// SendConfirmed, so repeated acks accumulate.
func (s *DeviceSession) recordAck(ackCode string, rtt time.Duration) {
	if s.store == nil || ackCode == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ack := Reception{At: time.Now().UTC(), RTTMs: rtt.Milliseconds()}
	if err := s.store.AppendAck(ctx, ackCode, ack); err != nil {
		Logf("record ack failed: %v", err)
	}
}

// markUnconfirmed marks a still-pending outgoing message as unconfirmed (the
// ack window elapsed without a confirmation).
func (s *DeviceSession) markUnconfirmed(ackCode string) {
	if s.store == nil || ackCode == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.MarkUnconfirmed(ctx, ackCode); err != nil {
		Logf("mark unconfirmed failed: %v", err)
	}
}

// storedInbox returns unread incoming messages from local state and marks them
// read. The radio inbox itself is drained separately by the drain loop, so this
// never touches the radio.
func (s *DeviceSession) storedInbox(ctx context.Context) ([]meshcore.Message, error) {
	if s.store == nil {
		return nil, nil
	}
	records, err := s.store.Messages(ctx, MessageFilter{Direction: MessageIn, UnreadOnly: true})
	if err != nil {
		return nil, err
	}
	out := make([]meshcore.Message, 0, len(records))
	ids := make([]int64, 0, len(records))
	for _, rec := range records {
		from := rec.Peer
		if rec.PeerName != "" {
			from = rec.PeerName
		}
		pathLen := 0
		if len(rec.Receptions) > 0 {
			pathLen = rec.Receptions[0].PathLen
		}
		out = append(out, meshcore.Message{
			From:      from,
			Channel:   rec.Channel,
			Text:      rec.Text,
			TxtType:   rec.TxtType,
			Timestamp: rec.Timestamp,
			SNR:       rec.SNR,
			PathLen:   byte(pathLen),
		})
		ids = append(ids, rec.ID)
	}
	if err := s.store.MarkMessagesRead(ctx, ids); err != nil {
		Logf("mark messages read failed: %v", err)
	}
	return out, nil
}
