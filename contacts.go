package meshcore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol"
	"github.com/meshcore-cz/meshcore-go/protocol/companion"
)

// ContactType classifies a contact by the kind of node it represents.
type ContactType string

const (
	ContactUnknown  ContactType = "unknown"
	ContactChat     ContactType = "chat"
	ContactRepeater ContactType = "repeater"
	ContactRoom     ContactType = "room"
	ContactSensor   ContactType = "sensor"

	contactStreamIdleTimeout = 5 * time.Second
)

func contactType(b byte) ContactType {
	switch b {
	case 1:
		return ContactChat
	case 2:
		return ContactRepeater
	case 3:
		return ContactRoom
	case 4:
		return ContactSensor
	default:
		return ContactUnknown
	}
}

// Contact identifies a peer known to the device.
type Contact struct {
	Name       string
	PublicKey  string // full hex-encoded public key (empty for event stubs)
	Type       ContactType
	HasPath    bool
	OutPathEnc byte   // encoded path metadata; 0xff = flood / unknown
	OutPath    []byte // raw out-path bytes when known
	Latitude   float64
	Longitude  float64
	LastAdvert time.Time
}

// ContactSyncProgress reports contact-list download progress from the radio.
type ContactSyncProgress struct {
	Received int
	Total    int // 0 until ContactsStart is received
}

// ContactSyncProgressFunc is called as contacts stream in from the radio.
type ContactSyncProgressFunc func(ContactSyncProgress)

// ContactSyncResult is the outcome of a contact synchronization stream.
type ContactSyncResult struct {
	Contacts []Contact
	LastMod  uint32
}

// Contacts returns the device's contact list.
func (c *Client) Contacts(ctx context.Context) ([]Contact, error) {
	result, err := c.ContactsSince(ctx, 0, nil)
	if err != nil {
		return nil, err
	}
	return result.Contacts, nil
}

// ContactsWithProgress returns the device's contact list and reports download
// progress when onProgress is non-nil.
func (c *Client) ContactsWithProgress(ctx context.Context, onProgress ContactSyncProgressFunc) ([]Contact, error) {
	result, err := c.ContactsSince(ctx, 0, onProgress)
	if err != nil {
		return nil, err
	}
	return result.Contacts, nil
}

// ContactsSince requests contacts changed since the given modification marker.
// Zero requests a full synchronization.
func (c *Client) ContactsSince(ctx context.Context, since uint32, onProgress ContactSyncProgressFunc) (ContactSyncResult, error) {
	if err := c.requireCapability(CapabilityContacts); err != nil {
		return ContactSyncResult{}, err
	}

	raw, err := c.proto.Encode(companion.GetContacts{Since: since})
	if err != nil {
		return ContactSyncResult{}, err
	}

	streamCtx, idle := streamIdleContext(ctx, contactStreamIdleTimeout)
	defer idle.cancel()

	var contacts []Contact
	var total int
	var lastMod uint32
	report := func() {
		if onProgress != nil {
			onProgress(ContactSyncProgress{Received: len(contacts), Total: total})
		}
	}
	collect := func(msg protocol.Message) (done bool) {
		idle.reset()
		switch m := msg.(type) {
		case companion.ContactsStart:
			total = int(m.Count)
			report()
		case companion.Contact:
			contacts = append(contacts, fromCompanionContact(m))
			report()
		case companion.EndOfContacts:
			lastMod = m.MostRecentLastMod
			return true
		}
		return false
	}

	if err := c.requestStream(streamCtx, raw, collect); err != nil {
		return ContactSyncResult{}, err
	}
	return ContactSyncResult{Contacts: contacts, LastMod: lastMod}, nil
}

// FindContact resolves a query against a contact list.
func FindContact(contacts []Contact, query string) (Contact, bool) {
	match, err := matchContact(contacts, query)
	if err != nil {
		return Contact{}, false
	}
	return match, true
}

// Contact looks up a single contact by name (case-insensitive) or by a
// public-key hex prefix. It performs a full device synchronization.
func (c *Client) Contact(ctx context.Context, name string) (Contact, error) {
	contacts, err := c.Contacts(ctx)
	if err != nil {
		return Contact{}, err
	}
	match, err := matchContact(contacts, name)
	if err != nil {
		return Contact{}, err
	}
	return match, nil
}

// matchContact resolves a query against a contact list, preferring an exact
// case-insensitive name match, then a unique name or key prefix.
func matchContact(contacts []Contact, query string) (Contact, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	var partial []Contact
	for _, ct := range contacts {
		name := strings.ToLower(ct.Name)
		if name == q || strings.EqualFold(ct.PublicKey, query) {
			return ct, nil
		}
		if strings.Contains(name, q) || strings.HasPrefix(strings.ToLower(ct.PublicKey), q) {
			partial = append(partial, ct)
		}
	}
	switch len(partial) {
	case 1:
		return partial[0], nil
	case 0:
		return Contact{}, fmt.Errorf("no contact matching %q", query)
	default:
		return Contact{}, fmt.Errorf("contact %q is ambiguous (%d matches)", query, len(partial))
	}
}

func fromCompanionContact(m companion.Contact) Contact {
	return Contact{
		Name:       m.Name,
		PublicKey:  m.PublicKey,
		Type:       contactType(m.Type),
		HasPath:    m.HasPath,
		OutPathEnc: m.OutPathEnc,
		OutPath:    cloneBytes(m.OutPath),
		Latitude:   m.Latitude,
		Longitude:  m.Longitude,
		LastAdvert: m.LastAdvert,
	}
}

func cloneBytes(b []byte) []byte {
	if len(b) == 0 {
		return []byte{}
	}
	return append([]byte(nil), b...)
}

type streamIdleHandle struct {
	cancel context.CancelFunc
	reset  func()
}

func streamIdleContext(parent context.Context, idle time.Duration) (context.Context, streamIdleHandle) {
	if idle <= 0 {
		return parent, streamIdleHandle{cancel: func() {}, reset: func() {}}
	}
	ctx, cancel := context.WithCancel(parent)
	var timer *time.Timer
	reset := func() {
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(idle, cancel)
	}
	reset()
	return ctx, streamIdleHandle{
		cancel: func() {
			if timer != nil {
				timer.Stop()
			}
			cancel()
		},
		reset: reset,
	}
}
