package meshcore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol/companion"
)

// RepeaterStats is binary status data from a remote repeater.
type RepeaterStats = companion.RepeaterStats

// RepeaterResponse is a text response returned by a remote repeater CLI.
type RepeaterResponse struct {
	Repeater   string
	Command    string
	Text       string               // plain-text CLI or sensor output
	Stats      *RepeaterStats       // populated for binary status responses
	Neighbours []RepeaterNeighbour  // populated for neighbors responses
	Received   time.Time
}

// RepeaterSession records a successful repeater login. It is a hint for when a
// session may still be active; the radio's connection table is authoritative.
type RepeaterSession struct {
	Repeater    string
	PublicKey   string
	LoggedInAt  time.Time
	ExpiresAt   time.Time
	Permissions byte
	Tag         int32
}

// Active reports whether the cached session has not passed its expiry hint.
func (s RepeaterSession) Active() bool {
	return !s.ExpiresAt.IsZero() && time.Now().Before(s.ExpiresAt)
}

// RepeaterHasConnection reports whether the companion radio still has an active
// login session to the repeater.
func (c *Client) RepeaterHasConnection(ctx context.Context, repeater string) (bool, error) {
	ct, err := c.repeaterContact(ctx, repeater)
	if err != nil {
		return false, err
	}
	return c.RepeaterHasConnectionContact(ctx, ct)
}

// RepeaterHasConnectionContact is like RepeaterHasConnection but uses a known
// contact instead of fetching the full contact list from the radio.
func (c *Client) RepeaterHasConnectionContact(ctx context.Context, ct Contact) (bool, error) {
	_, key, err := repeaterKeyFromContact(ct)
	if err != nil {
		return false, err
	}
	c.log.Debug("radio send", "op", "has_connection", "cmd", "0x1c", "repeater", ct.Name, "key_prefix", hex.EncodeToString(key[:6]), "source", "local_replica")
	msg, err := c.request(ctx, companion.HasConnection{PublicKey: key})
	if err != nil {
		var de *DeviceError
		if errors.As(err, &de) && de.Code == companion.ErrNotFound {
			c.log.Debug("radio recv", "op", "has_connection", "response", "not_found", "err_code", de.Code)
			return false, nil
		}
		c.log.Debug("radio recv", "op", "has_connection", "error", err)
		return false, err
	}
	if _, ok := msg.(companion.OK); ok {
		c.log.Debug("radio recv", "op", "has_connection", "response", "ok")
		return true, nil
	}
	c.log.Debug("radio recv", "op", "has_connection", "response", fmt.Sprintf("%T", msg))
	return false, fmt.Errorf("meshcore: unexpected has_connection response: %T", msg)
}

// RepeaterLogin logs in to a remote repeater using its saved contact key.
func (c *Client) RepeaterLogin(ctx context.Context, repeater, password string) (RepeaterSession, error) {
	ct, err := c.repeaterContact(ctx, repeater)
	if err != nil {
		return RepeaterSession{}, err
	}
	return c.RepeaterLoginContact(ctx, ct, password)
}

// RepeaterLoginContact is like RepeaterLogin but uses a known contact instead
// of fetching the full contact list from the radio.
func (c *Client) RepeaterLoginContact(ctx context.Context, ct Contact, password string) (RepeaterSession, error) {
	if err := c.requireCapability(CapabilityRepeaterLogin); err != nil {
		return RepeaterSession{}, err
	}
	ct, key, err := repeaterKeyFromContact(ct)
	if err != nil {
		return RepeaterSession{}, err
	}
	c.log.Debug("radio send", "op", "send_login", "cmd", "0x1a", "repeater", ct.Name, "key_prefix", hex.EncodeToString(key[:6]), "password_len", len(password), "source", "local_replica")
	msg, err := c.request(ctx, companion.SendLogin{
		PublicKey: key,
		Password:  password,
	})
	if err != nil {
		c.log.Debug("radio recv", "op", "send_login", "error", err)
		return RepeaterSession{}, err
	}
	receipt, err := receiptFrom(msg, ct.Name)
	if err != nil {
		c.log.Debug("radio recv", "op", "send_login", "response", fmt.Sprintf("%T", msg), "error", err)
		return RepeaterSession{}, err
	}
	timeout := receipt.Timeout
	if timeout == 0 {
		timeout = c.timeout * 3
	}
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	c.log.Debug("radio recv", "op", "send_login", "response", "sent", "timeout", timeout)
	c.log.Debug("waiting for login push", "repeater", ct.Name, "timeout", timeout)
	success, err := c.waitForRepeaterLogin(ctx, ct, key[:6], timeout)
	if err != nil {
		return RepeaterSession{}, err
	}
	sess := newRepeaterSession(ct, success, receipt.Timeout)
	c.log.Debug("repeater login ok", "name", ct.Name, "expires_at", sess.ExpiresAt)
	return sess, nil
}

func newRepeaterSession(ct Contact, success RepeaterLoginSucceeded, estTimeout time.Duration) RepeaterSession {
	now := time.Now()
	ttl := estTimeout
	if ttl < 15*time.Minute {
		ttl = 30 * time.Minute
	}
	if ttl > 2*time.Hour {
		ttl = 2 * time.Hour
	}
	return RepeaterSession{
		Repeater:    ct.Name,
		PublicKey:   ct.PublicKey,
		LoggedInAt:  now,
		ExpiresAt:   now.Add(ttl),
		Permissions: success.Permissions,
		Tag:         success.Tag,
	}
}

func (c *Client) waitForRepeaterLogin(ctx context.Context, ct Contact, prefix []byte, timeout time.Duration) (RepeaterLoginSucceeded, error) {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-wctx.Done():
			return RepeaterLoginSucceeded{}, fmt.Errorf("waiting for repeater %s login: %w", ct.Name, wctx.Err())
		case <-c.closed:
			return RepeaterLoginSucceeded{}, fmt.Errorf("meshcore: connection closed")
		case ev, ok := <-c.events.Events():
			if !ok {
				return RepeaterLoginSucceeded{}, fmt.Errorf("meshcore: event stream closed")
			}
			switch m := ev.(type) {
			case RepeaterLoginSucceeded:
				if loginPrefixMatches(prefix, m.PublicKeyPrefix, ct.Name) {
					c.log.Debug("radio recv", "op", "send_login", "response", "login_success", "repeater", ct.Name)
					return m, nil
				}
			case RepeaterLoginFailed:
				if loginPrefixMatches(prefix, m.PublicKeyPrefix, ct.Name) {
					c.log.Debug("radio recv", "op", "send_login", "response", "login_fail", "repeater", ct.Name)
					return RepeaterLoginSucceeded{}, fmt.Errorf("repeater %s login failed", ct.Name)
				}
			}
		}
	}
}

func loginPrefixMatches(prefix []byte, evPrefix, name string) bool {
	if len(prefix) == 0 {
		return false
	}
	want := hex.EncodeToString(prefix)
	return strings.EqualFold(evPrefix, want) ||
		strings.EqualFold(evPrefix, name) ||
		(len(evPrefix) >= len(want) && strings.EqualFold(evPrefix[:len(want)], want))
}

// RepeaterStatus requests binary stats from a remote repeater.
func (c *Client) RepeaterStatus(ctx context.Context, repeater string) (RepeaterResponse, error) {
	ct, err := c.repeaterContact(ctx, repeater)
	if err != nil {
		return RepeaterResponse{}, err
	}
	return c.RepeaterStatusContact(ctx, ct)
}

// RepeaterStatusContact is like RepeaterStatus but uses a known contact instead
// of fetching the full contact list from the radio.
func (c *Client) RepeaterStatusContact(ctx context.Context, ct Contact) (RepeaterResponse, error) {
	if err := c.requireCapability(CapabilityRepeaterCommands); err != nil {
		return RepeaterResponse{}, err
	}
	ct, key, err := repeaterKeyFromContact(ct)
	if err != nil {
		return RepeaterResponse{}, err
	}
	statusStart := time.Now()
	c.log.Debug("repeater status start", "name", ct.Name, "started_at", statusStart.Format(time.RFC3339Nano), "public_key", ct.PublicKey[:min(12, len(ct.PublicKey))])
	c.log.Debug("radio send", "op", "send_status_req", "cmd", "0x1b", "repeater", ct.Name, "key_prefix", hex.EncodeToString(key[:6]))

	msg, err := c.request(ctx, companion.SendStatusReq{PublicKey: key})
	if err != nil {
		c.log.Debug("radio recv", "op", "send_status_req", "error", err, "elapsed", time.Since(statusStart).Round(time.Millisecond))
		return RepeaterResponse{}, err
	}
	receipt, err := receiptFrom(msg, ct.Name)
	if err != nil {
		c.log.Debug("radio recv", "op", "send_status_req", "response", fmt.Sprintf("%T", msg), "error", err)
		return RepeaterResponse{}, err
	}
	timeout := receipt.Timeout
	if timeout == 0 {
		timeout = c.timeout * 3
	}
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	c.log.Debug("radio recv", "op", "send_status_req", "response", "sent", "timeout", timeout, "elapsed", time.Since(statusStart).Round(time.Millisecond))
	waitStart := time.Now()
	c.log.Debug("waiting for status push", "repeater", ct.Name, "timeout", timeout, "started_at", waitStart.Format(time.RFC3339Nano))
	stats, text, err := c.waitForRepeaterStatus(ctx, ct, key[:6], timeout)
	if err != nil {
		c.log.Debug("repeater status failed", "name", ct.Name, "error", err, "wait_elapsed", time.Since(waitStart).Round(time.Millisecond), "total_elapsed", time.Since(statusStart).Round(time.Millisecond))
		return RepeaterResponse{}, err
	}
	c.log.Debug("repeater status ok", "name", ct.Name, "stats", stats != nil, "bytes", len(text), "wait_elapsed", time.Since(waitStart).Round(time.Millisecond), "total_elapsed", time.Since(statusStart).Round(time.Millisecond))
	return RepeaterResponse{Repeater: ct.Name, Command: "status", Stats: stats, Text: text, Received: time.Now()}, nil
}

func (c *Client) waitForRepeaterStatus(ctx context.Context, ct Contact, prefix []byte, timeout time.Duration) (*RepeaterStats, string, error) {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-wctx.Done():
			return nil, "", fmt.Errorf("waiting for repeater %s status: %w", ct.Name, wctx.Err())
		case <-c.closed:
			return nil, "", fmt.Errorf("meshcore: connection closed")
		case ev, ok := <-c.events.Events():
			if !ok {
				return nil, "", fmt.Errorf("meshcore: event stream closed")
			}
			switch m := ev.(type) {
			case RepeaterStatusReceived:
				if loginPrefixMatches(prefix, m.PublicKeyPrefix, ct.Name) {
					c.log.Debug("radio recv", "op", "send_status_req", "response", "status_push", "repeater", ct.Name, "stats", m.Stats != nil, "bytes", len(m.Text), "at", time.Now().Format(time.RFC3339Nano))
					return m.Stats, m.Text, nil
				}
				c.log.Debug("repeater status ignored", "name", ct.Name, "from", m.PublicKeyPrefix, "at", time.Now().Format(time.RFC3339Nano))
			default:
				c.log.Debug("repeater status event", "name", ct.Name, "type", fmt.Sprintf("%T", ev))
			}
		}
	}
}

// RepeaterNeighbours requests the repeater's CLI neighbour list.
func (c *Client) RepeaterNeighbours(ctx context.Context, repeater string) (RepeaterResponse, error) {
	ct, err := c.repeaterContact(ctx, repeater)
	if err != nil {
		return RepeaterResponse{}, err
	}
	return c.RepeaterNeighboursContact(ctx, ct)
}

// RepeaterNeighboursContact is like RepeaterNeighbours but uses a known contact.
func (c *Client) RepeaterNeighboursContact(ctx context.Context, ct Contact) (RepeaterResponse, error) {
	resp, err := c.RepeaterExecContact(ctx, ct, "neighbors")
	if err != nil {
		return resp, err
	}
	resp.Neighbours = ParseRepeaterNeighbours(resp.Text)
	return resp, nil
}

// RepeaterExec sends a command to a remote repeater CLI and waits for a text
// response from the same contact.
func (c *Client) RepeaterExec(ctx context.Context, repeater, command string) (RepeaterResponse, error) {
	ct, err := c.repeaterContact(ctx, repeater)
	if err != nil {
		return RepeaterResponse{}, err
	}
	return c.RepeaterExecContact(ctx, ct, command)
}

// RepeaterExecContact is like RepeaterExec but uses a known contact instead of
// fetching the full contact list from the radio.
func (c *Client) RepeaterExecContact(ctx context.Context, ct Contact, command string) (RepeaterResponse, error) {
	if err := c.requireCapability(CapabilityRepeaterCommands); err != nil {
		return RepeaterResponse{}, err
	}
	ct, key, err := repeaterKeyFromContact(ct)
	if err != nil {
		return RepeaterResponse{}, err
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return RepeaterResponse{}, fmt.Errorf("repeater command is empty")
	}
	c.log.Debug("repeater exec", "name", ct.Name, "command", command)

	msg, err := c.request(ctx, companion.SendTextMessage{
		DestPublicKey: key,
		Text:          command,
		TxtType:       1, // remote CLI command
	})
	if err != nil {
		return RepeaterResponse{}, err
	}
	receipt, err := receiptFrom(msg, ct.Name)
	if err != nil {
		c.log.Debug("repeater exec unexpected response", "name", ct.Name, "command", command, "type", fmt.Sprintf("%T", msg))
		return RepeaterResponse{}, err
	}
	timeout := receipt.Timeout
	if timeout == 0 {
		timeout = c.timeout * 3
	}
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	c.log.Debug("repeater exec queued", "name", ct.Name, "command", command, "timeout", timeout)
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	keyPrefix := hex.EncodeToString(key[:6])
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wctx.Done():
			return RepeaterResponse{}, fmt.Errorf("waiting for repeater %s response to %q: %w", ct.Name, command, wctx.Err())
		case <-c.closed:
			return RepeaterResponse{}, fmt.Errorf("meshcore: connection closed")
		case ev, ok := <-c.events.Events():
			if !ok {
				return RepeaterResponse{}, fmt.Errorf("meshcore: event stream closed")
			}
			if msg, ok := ev.(MessageReceived); ok {
				if messageMatchesRepeaterCLI(msg, ct, keyPrefix) {
					c.log.Debug("repeater exec response", "name", ct.Name, "command", command, "bytes", len(msg.Text), "source", "event")
					return RepeaterResponse{Repeater: ct.Name, Command: command, Text: msg.Text, Received: msg.Timestamp}, nil
				}
				c.log.Debug("repeater exec ignored message", "name", ct.Name, "command", command, "from", msg.From.Name, "txt_type", msg.TxtType)
				continue
			}
			c.log.Debug("repeater exec event", "name", ct.Name, "command", command, "type", fmt.Sprintf("%T", ev))
		case <-ticker.C:
			c.log.Debug("repeater exec sync poll", "name", ct.Name, "command", command)
			msgs, err := c.SyncMessages(wctx)
			if err != nil {
				c.log.Debug("repeater exec sync failed", "name", ct.Name, "command", command, "error", err)
				continue
			}
			c.log.Debug("repeater exec sync", "name", ct.Name, "command", command, "messages", len(msgs))
			for _, msg := range msgs {
				if messageMatchesRepeaterCLIMessage(msg, ct, keyPrefix) {
					c.log.Debug("repeater exec response", "name", ct.Name, "command", command, "bytes", len(msg.Text), "source", "sync")
					return RepeaterResponse{Repeater: ct.Name, Command: command, Text: msg.Text, Received: msg.Timestamp}, nil
				}
				c.log.Debug("repeater exec ignored sync message", "name", ct.Name, "command", command, "from", msg.From, "txt_type", msg.TxtType)
			}
		}
	}
}

func (c *Client) repeaterContact(ctx context.Context, repeater string) (Contact, error) {
	lookupStart := time.Now()
	c.log.Debug("radio send", "op", "contact_lookup", "repeater", repeater, "source", "device_get_contacts")
	ct, err := c.Contact(ctx, repeater)
	if err != nil {
		c.log.Debug("radio recv", "op", "contact_lookup", "repeater", repeater, "error", err, "elapsed", time.Since(lookupStart).Round(time.Millisecond))
		return Contact{}, err
	}
	c.log.Debug("radio recv", "op", "contact_lookup", "repeater", ct.Name, "type", ct.Type, "has_path", ct.HasPath, "elapsed", time.Since(lookupStart).Round(time.Millisecond))
	return ct, nil
}

func repeaterKeyFromContact(ct Contact) (Contact, []byte, error) {
	if ct.Type != ContactRepeater && ct.Type != ContactRoom && ct.Type != ContactSensor {
		return Contact{}, nil, fmt.Errorf("%q is %s, not a repeater/room/sensor", ct.Name, ct.Type)
	}
	key, err := hex.DecodeString(ct.PublicKey)
	if err != nil || len(key) < 32 {
		return Contact{}, nil, fmt.Errorf("repeater %q has no usable public key", ct.Name)
	}
	return ct, key, nil
}

const repeaterCLITxtType byte = 1 // TXT_TYPE_CLI_DATA

func messageMatchesRepeaterCLI(msg MessageReceived, ct Contact, keyPrefix string) bool {
	if msg.TxtType != repeaterCLITxtType {
		return false
	}
	return repeaterSenderMatches(msg.From.Name, ct, keyPrefix)
}

func messageMatchesRepeaterCLIMessage(msg Message, ct Contact, keyPrefix string) bool {
	if msg.TxtType != repeaterCLITxtType {
		return false
	}
	return repeaterSenderMatches(msg.From, ct, keyPrefix)
}

func repeaterSenderMatches(from string, ct Contact, keyPrefix string) bool {
	from = strings.TrimSpace(from)
	if strings.EqualFold(from, ct.Name) {
		return true
	}
	if keyPrefix != "" && strings.EqualFold(from, keyPrefix) {
		return true
	}
	if keyPrefix != "" && len(from) >= 6 && strings.HasPrefix(strings.ToLower(ct.PublicKey), strings.ToLower(from)) {
		return true
	}
	return false
}
