package cli

import (
	"context"
	"fmt"
)

// cmdChannel implements `mcr channel list|show|send`.
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
	client, _, err := connect(ctx, e)
	if err != nil {
		return err
	}
	defer client.Close()

	channels, err := client.Channels(ctx)
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
		return fmt.Errorf("usage: mcr channel show <name|index>")
	}
	client, _, err := connect(ctx, e)
	if err != nil {
		return err
	}
	defer client.Close()

	ch, err := client.Channel(ctx, name)
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
		return fmt.Errorf("usage: mcr channel send <name|index> <text>")
	}
	client, _, err := connect(ctx, e)
	if err != nil {
		return err
	}
	defer client.Close()

	receipt, err := client.SendChannelText(ctx, channel, text)
	if err != nil {
		return err
	}
	e.out.Human("Sent to %s.\n", receipt.To)
	return e.out.JSONValue(map[string]any{"to": receipt.To, "id": receipt.ID()})
}
