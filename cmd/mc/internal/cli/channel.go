package cli

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

// cmdChannel implements `mc channel list|show|send|add|remove`.
func cmdChannel(ctx context.Context, e *env) error {
	switch e.restArg(0) {
	case "", "list":
		return channelList(ctx, e)
	case "show":
		return channelShow(ctx, e)
	case "send":
		return channelSend(ctx, e)
	case "add":
		return channelAdd(ctx, e)
	case "remove", "rm", "del":
		return channelRemove(ctx, e)
	default:
		return fmt.Errorf("unknown channel subcommand %q", e.restArg(0))
	}
}

// channelAdd implements `mc channel add <name> [key]`. With no key, a #name
// hash channel derives its key from the name; otherwise a 16-byte key is
// required (hex or base64). The channel is written to the device, then local
// state is re-synced from the device.
func channelAdd(ctx context.Context, e *env) error {
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: mc channel add <name> [key]")
	}
	keyArg := e.restArg(2)

	// Public and hashtag channels derive their key from the full name including
	// the leading '#', and are stored with the '#' (the name is the key).
	hashChannel := strings.HasPrefix(name, "#")

	var secret []byte
	switch {
	case keyArg != "":
		var err error
		secret, err = parseChannelKey(keyArg)
		if err != nil {
			return err
		}
	case hashChannel:
		secret = meshcore.DeriveChannelSecret(name)
	default:
		return fmt.Errorf("provide a 16-byte key, or use a #name hash channel to derive one")
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	ch, err := backend.ChannelAdd(ctx, name, secret)
	if err != nil {
		return err
	}
	keyHex := hex.EncodeToString(secret)
	e.out.Human("Added channel %s (slot %d).\n", meshcore.ChannelDisplayName(ch.Name), ch.Index)
	e.out.Human("Key: %s\n", keyHex)
	return e.out.JSONValue(map[string]any{"name": ch.Name, "index": ch.Index, "key": keyHex})
}

// channelRemove implements `mc channel remove <name|index>`. It removes the
// channel from the device first, then re-syncs local state.
func channelRemove(ctx context.Context, e *env) error {
	channel := e.restArg(1)
	if channel == "" {
		return fmt.Errorf("usage: mc channel remove <name|index>")
	}
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	ch, err := backend.ChannelRemove(ctx, channel)
	if err != nil {
		return err
	}
	e.out.Human("Removed channel %s (slot %d).\n", meshcore.ChannelDisplayName(ch.Name), ch.Index)
	return e.out.JSONValue(map[string]any{"removed": ch.Name, "index": ch.Index})
}

// parseChannelKey accepts a 16-byte channel key as hex (32 chars) or base64.
func parseChannelKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if len(s) == 2*meshcore.ChannelSecretLen {
		if b, err := hex.DecodeString(s); err == nil {
			return b, nil
		}
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) == meshcore.ChannelSecretLen {
		return b, nil
	}
	if b, err := hex.DecodeString(s); err == nil && len(b) == meshcore.ChannelSecretLen {
		return b, nil
	}
	return nil, fmt.Errorf("channel key must be %d bytes as hex or base64", meshcore.ChannelSecretLen)
}

func channelList(ctx context.Context, e *env) error {
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	channels, err := backend.ChannelsWithOptions(ctx, e.args.has("refresh"))
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(channels)
	}
	if len(channels) == 0 {
		e.out.Human("No channels.\n")
		return nil
	}
	e.out.Human("%-6s %s\n", "INDEX", "NAME")
	for _, ch := range channels {
		e.out.Human("%-6d %s\n", ch.Index, ch.Name)
	}
	return nil
}

func channelShow(ctx context.Context, e *env) error {
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: mc channel show <name|index>")
	}
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	ch, err := backend.Channel(ctx, name)
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(ch)
	}
	e.out.Human("Index: %d\n", ch.Index)
	e.out.Human("Name:  %s\n", ch.Name)
	return nil
}

func channelSend(ctx context.Context, e *env) error {
	channel := e.restArg(1)
	text := e.restArg(2)
	if channel == "" || text == "" {
		return fmt.Errorf("usage: mc channel send <name|index> <text>")
	}
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	receipt, err := backend.SendChannelText(ctx, channel, text)
	if err != nil {
		return err
	}
	e.out.Human("Sent to %s.\n", receipt.To)
	return e.out.JSONValue(map[string]any{"to": receipt.To, "id": receipt.ID()})
}
