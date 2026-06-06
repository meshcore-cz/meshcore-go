package meshcore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meshcore-dev/meshcore-go/protocol"
	"github.com/meshcore-dev/meshcore-go/protocol/companion"
)

// ContactType classifies a contact by the kind of node it represents.
type ContactType string

const (
	ContactUnknown  ContactType = "unknown"
	ContactChat     ContactType = "chat"
	ContactRepeater ContactType = "repeater"
	ContactRoom     ContactType = "room"
	ContactSensor   ContactType = "sensor"
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
	Latitude   float64
	Longitude  float64
	LastAdvert time.Time
}

// Contacts returns the device's contact list.
func (c *Client) Contacts(ctx context.Context) ([]Contact, error) {
	if err := c.requireCapability(CapabilityContacts); err != nil {
		return nil, err
	}

	raw, err := c.proto.Encode(companion.GetContacts{})
	if err != nil {
		return nil, err
	}

	var contacts []Contact
	collect := func(msg protocol.Message) (done bool) {
		switch m := msg.(type) {
		case companion.Contact:
			contacts = append(contacts, fromCompanionContact(m))
		case companion.EndOfContacts:
			return true
		}
		return false
	}

	if err := c.requestStream(ctx, raw, collect); err != nil {
		return nil, err
	}
	return contacts, nil
}

// Contact looks up a single contact by name (case-insensitive) or by a
// public-key hex prefix.
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
		Latitude:   m.Latitude,
		Longitude:  m.Longitude,
		LastAdvert: m.LastAdvert,
	}
}
