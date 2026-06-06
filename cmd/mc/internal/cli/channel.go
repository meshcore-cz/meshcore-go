package cli

import (
	"context"
	"fmt"
)

// cmdChannel implements `mc channel list|show|send`.
func cmdChannel(ctx context.Context, e *env) error {
	switch e.restArg(0) {
	case "", "list":
		return channelList(ctx, e)
	case "show":
		return channelShow(ctx, e)
	case "send":
		return channelSend(ctx, e)
	default:
		return fmt.Errorf("unknown channel subcommand %q", e.restArg(0))
	}
}

func channelList(ctx context.Context, e *env) error {
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	channels, err := backend.Channels(ctx)
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
