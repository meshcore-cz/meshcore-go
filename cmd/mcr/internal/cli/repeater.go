package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	meshcore "github.com/meshcore-dev/meshcore-go"
	"github.com/meshcore-dev/meshcore-go/cmd/mcr/internal/config"
)

func cmdRepeater(ctx context.Context, e *env) error {
	switch e.restArg(0) {
	case "add":
		return repeaterAdd(ctx, e)
	case "status":
		return repeaterStatus(ctx, e)
	case "neighbours", "neighbors":
		return repeaterNeighbours(ctx, e)
	case "exec":
		return repeaterExec(ctx, e)
	default:
		return fmt.Errorf("usage: mcr repeater <add|status|neighbours|exec>")
	}
}

func repeaterAdd(ctx context.Context, e *env) error {
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: mcr repeater add <name> [password]")
	}
	password := e.restArg(2)
	if password == "" && !e.out.JSON {
		password = promptSecret("Repeater password")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.PutRepeater(name, config.Repeater{Name: name, Password: password}, true)
	if err := cfg.Save(); err != nil {
		return err
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()
	if password != "" {
		if err := backend.RepeaterLogin(ctx, name, password); err != nil {
			return err
		}
		e.out.Human("Saved and logged in to repeater %q.\n", name)
	} else {
		e.out.Human("Saved repeater %q without a password.\n", name)
	}
	return e.out.JSONValue(map[string]any{"name": name, "saved": true, "logged_in": password != ""})
}

func repeaterStatus(ctx context.Context, e *env) error {
	name, err := resolveRepeaterArg(e, 1)
	if err != nil {
		return err
	}
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()
	if err := loginSavedRepeater(ctx, backend, name); err != nil {
		return err
	}
	resp, err := backend.RepeaterStatus(ctx, name)
	if err != nil {
		return err
	}
	return printRepeaterResponse(e, resp)
}

func repeaterNeighbours(ctx context.Context, e *env) error {
	name, err := resolveRepeaterArg(e, 1)
	if err != nil {
		return err
	}
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()
	if err := loginSavedRepeater(ctx, backend, name); err != nil {
		return err
	}
	resp, err := backend.RepeaterNeighbours(ctx, name)
	if err != nil {
		return err
	}
	return printRepeaterResponse(e, resp)
}

func repeaterExec(ctx context.Context, e *env) error {
	name, start, err := resolveRepeater(e, 1)
	if err != nil {
		return err
	}
	command := strings.Join(e.rest[start:], " ")
	if command == "" {
		return fmt.Errorf("usage: mcr repeater exec [name] <command>")
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()
	if err := loginSavedRepeater(ctx, backend, name); err != nil {
		return err
	}
	resp, err := backend.RepeaterExec(ctx, name, command)
	if err != nil {
		return err
	}
	return printRepeaterResponse(e, resp)
}

func resolveRepeater(e *env, start int) (name string, next int, err error) {
	cfg, err := config.Load()
	if err != nil {
		return "", start, err
	}
	arg := e.restArg(start)
	if arg != "" {
		if _, ok := cfg.Repeaters[arg]; ok {
			return arg, start + 1, nil
		}
		if cfg.CurrentRepeater == "" || e.restArg(start+1) != "" {
			return arg, start + 1, nil
		}
	}
	if cfg.CurrentRepeater != "" {
		return cfg.CurrentRepeater, start, nil
	}
	if len(cfg.Repeaters) == 1 {
		for n := range cfg.Repeaters {
			return n, start, nil
		}
	}
	return "", start, fmt.Errorf("no repeater selected; run `mcr repeater add <name>` or pass a repeater name")
}

func resolveRepeaterArg(e *env, start int) (string, error) {
	if arg := e.restArg(start); arg != "" {
		return arg, nil
	}
	name, _, err := resolveRepeater(e, start)
	return name, err
}

func loginSavedRepeater(ctx context.Context, backend Backend, name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rep, ok := cfg.Repeaters[name]
	if !ok || rep.Password == "" {
		return nil
	}
	return backend.RepeaterLogin(ctx, name, rep.Password)
}

func printRepeaterResponse(e *env, resp meshcore.RepeaterResponse) error {
	if e.out.JSON {
		return e.out.JSONValue(resp)
	}
	e.out.Human("%s\n", resp.Text)
	return nil
}

func promptSecret(label string) string {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}
