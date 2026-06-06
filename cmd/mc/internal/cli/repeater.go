package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
)

func cmdRepeater(ctx context.Context, e *env) error {
	switch e.restArg(0) {
	case "", "list":
		return repeaterList(e)
	case "add":
		return repeaterAdd(ctx, e)
	case "del", "delete", "remove":
		return repeaterDel(e)
	case "status":
		return repeaterStatus(ctx, e)
	case "neighbours", "neighbors":
		return repeaterNeighbours(ctx, e)
	case "exec":
		return repeaterExec(ctx, e)
	default:
		return fmt.Errorf("usage: mc repeater <list|add|del|status|neighbours|exec>")
	}
}

func repeaterList(e *env) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(cfg.Repeaters))
	for key := range cfg.Repeaters {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, nj := cfg.Repeaters[keys[i]].Name, cfg.Repeaters[keys[j]].Name
		if ni == "" {
			ni = keys[i]
		}
		if nj == "" {
			nj = keys[j]
		}
		return ni < nj
	})

	if e.out.JSON {
		type row struct {
			Name        string `json:"name"`
			PublicKey   string `json:"public_key"`
			HasPassword bool   `json:"has_password"`
			Default     bool   `json:"default"`
		}
		rows := make([]row, 0, len(keys))
		for _, key := range keys {
			rep := cfg.Repeaters[key]
			name := rep.Name
			if name == "" {
				name = key
			}
			rows = append(rows, row{
				Name:        name,
				PublicKey:   config.NormalizePublicKey(key),
				HasPassword: rep.Password != "",
				Default:     config.NormalizePublicKey(key) == config.NormalizePublicKey(cfg.CurrentRepeater),
			})
		}
		return e.out.JSONValue(rows)
	}

	if len(keys) == 0 {
		e.out.Human("No saved repeaters. Run `mc repeater add <name>`.\n")
		return nil
	}
	e.out.Human("%-26s %-14s %-9s %s\n", "NAME", "PUBLIC KEY", "PASSWORD", "DEFAULT")
	for _, key := range keys {
		rep := cfg.Repeaters[key]
		name := rep.Name
		if name == "" {
			name = key
		}
		password := "-"
		if rep.Password != "" {
			password = "saved"
		}
		def := ""
		if config.NormalizePublicKey(key) == config.NormalizePublicKey(cfg.CurrentRepeater) {
			def = "*"
		}
		e.out.Human("%-26s %-14s %-9s %s\n", name, shortKey(key), password, def)
	}
	return nil
}

func repeaterDel(e *env) error {
	name, err := resolveRepeaterArg(e, 1)
	if err != nil {
		return fmt.Errorf("usage: mc repeater del [name]")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	key, rep, ok := cfg.RemoveRepeater(name)
	if !ok {
		return fmt.Errorf("unknown repeater %q", name)
	}
	displayName := repeaterContactName(name, rep)
	if err := config.RemoveRepeaterSession(key); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	e.dbg.Log("repeater removed", "name", displayName, "public_key", shortKey(key))
	if !e.out.JSON {
		e.out.Human("Removed repeater %q.\n", displayName)
	}
	return e.out.JSONValue(map[string]any{
		"removed":    displayName,
		"public_key": config.NormalizePublicKey(key),
	})
}

func repeaterAdd(ctx context.Context, e *env) error {
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: mc repeater add <name> [password]")
	}
	password := e.restArg(2)
	if password == "" && !e.out.JSON {
		password = promptSecret("Repeater password")
	}

	e.dbg.Log("repeater add", "name", name, "has_password", password != "")

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	contactStart := time.Now()
	e.dbg.SendCommand("contact_lookup", 0, "name", name)
	ct, err := backend.Contact(ctx, name)
	e.dbg.CommandDone("contact_lookup", contactStart, "error", err)
	if err != nil {
		return fmt.Errorf("lookup contact %q: %w", name, err)
	}
	e.dbg.Contact(ct)

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if oldKey, _, ok := cfg.MatchRepeater(name); ok && oldKey != ct.PublicKey {
		delete(cfg.Repeaters, oldKey)
	}
	cfg.PutRepeater(ct.PublicKey, config.Repeater{Name: ct.Name, Password: password}, true)
	if err := cfg.Save(); err != nil {
		return err
	}
	if path, err := config.Path(); err == nil {
		e.dbg.Log("repeater saved", "name", ct.Name, "public_key", shortKey(ct.PublicKey), "config", path, "current", cfg.CurrentRepeater)
	}

	if password != "" {
		sess, err := ensureRepeaterLogin(ctx, e, backend, ct, password, false)
		if err != nil {
			return err
		}
		e.dbg.Log("repeater login ok", "name", ct.Name, "expires_at", sess.ExpiresAt)
		e.out.Human("Saved and logged in to repeater %q.\n", ct.Name)
	} else {
		e.dbg.Log("skipping login", "name", ct.Name, "reason", "no password")
		e.out.Human("Saved repeater %q without a password.\n", ct.Name)
	}
	return e.out.JSONValue(map[string]any{
		"name":       ct.Name,
		"public_key": ct.PublicKey,
		"saved":      true,
		"logged_in":  password != "",
	})
}

func repeaterStatus(ctx context.Context, e *env) error {
	return runRepeaterQuery(ctx, e, "status", 1, func(ctx context.Context, b Backend, name string) (meshcore.RepeaterResponse, error) {
		return b.RepeaterStatus(ctx, name)
	})
}

func repeaterNeighbours(ctx context.Context, e *env) error {
	return runRepeaterQuery(ctx, e, "neighbours", 1, func(ctx context.Context, b Backend, name string) (meshcore.RepeaterResponse, error) {
		return b.RepeaterNeighbours(ctx, name)
	})
}

func repeaterExec(ctx context.Context, e *env) error {
	name, start, err := resolveRepeater(e, 1)
	if err != nil {
		return err
	}
	command := strings.Join(e.rest[start:], " ")
	if command == "" {
		return fmt.Errorf("usage: mc repeater exec [name] <command>")
	}
	return runRepeaterQuery(ctx, e, "exec", -1, func(ctx context.Context, b Backend, _ string) (meshcore.RepeaterResponse, error) {
		return b.RepeaterExec(ctx, name, command)
	}, withRepeaterName(name), withRepeaterCommand(command))
}

type repeaterQueryOption func(*repeaterQueryOpts)

type repeaterQueryOpts struct {
	name    string
	command string
}

func withRepeaterName(name string) repeaterQueryOption {
	return func(o *repeaterQueryOpts) { o.name = name }
}

func withRepeaterCommand(command string) repeaterQueryOption {
	return func(o *repeaterQueryOpts) { o.command = command }
}

type repeaterQueryFunc func(context.Context, Backend, string) (meshcore.RepeaterResponse, error)

func runRepeaterQuery(ctx context.Context, e *env, op string, nameIndex int, query repeaterQueryFunc, opts ...repeaterQueryOption) error {
	var qo repeaterQueryOpts
	for _, opt := range opts {
		opt(&qo)
	}

	name := qo.name
	if name == "" {
		var err error
		name, err = resolveRepeaterArg(e, nameIndex)
		if err != nil {
			return err
		}
	}
	cmdStart := time.Now()
	e.dbg.Started("repeater "+op, cmdStart, "name", name, "command", qo.command)

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()
	e.dbg.Phase("repeater "+op, "backend_open", cmdStart, "uri", backend.URI(), "transport", backend.Transport())

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if sess, ok := config.CachedRepeaterSession(cfg, name); ok {
		e.dbg.Phase("repeater "+op, "session_cached", cmdStart,
			"name", sess.Name,
			"public_key", shortKey(sess.PublicKey),
			"expires_at", sess.ExpiresAt,
		)
	} else {
		contactStart := time.Now()
		e.dbg.SendCommand("contact_lookup", 0, "name", name)
		if ct, err := backend.Contact(ctx, name); err != nil {
			e.dbg.CommandDone("contact_lookup", contactStart, "error", err)
			e.dbg.Log("contact lookup failed", "name", name, "error", err)
		} else {
			e.dbg.CommandDone("contact_lookup", contactStart)
			e.dbg.Contact(ct)
		}
		loginStart := time.Now()
		e.dbg.Log("repeater login check", "name", name, "at", loginStart.Format("2006-01-02 15:04:05.000"))
		if err := loginSavedRepeater(ctx, e, backend, name, false); err != nil {
			return err
		}
		e.dbg.Phase("repeater "+op, "login_check", cmdStart, "elapsed_login", time.Since(loginStart).Round(time.Millisecond))
	}

	switch op {
	case "status":
		e.dbg.SendCommand("send_status_req", 0x1b, "repeater", name)
		e.dbg.Log("waiting for status push", "repeater", name)
	case "neighbours":
		e.dbg.SendCommand("repeater_exec", 0x02, "repeater", name, "command", "neighbors")
	case "exec":
		e.dbg.SendCommand("repeater_exec", 0x02, "repeater", name, "command", qo.command)
	}

	queryStart := time.Now()
	resp, err := query(ctx, backend, name)
	e.dbg.CommandDone(op, queryStart, "error", err, "stats", resp.Stats != nil, "bytes", len(resp.Text))
	if err != nil {
		e.dbg.Log("repeater "+op+" failed", "name", name, "error", err, "at", time.Now().Format("2006-01-02 15:04:05.000"))
		if reloginErr := loginSavedRepeater(ctx, e, backend, name, true); reloginErr != nil {
			return err
		}
		e.dbg.Log("repeater "+op+" retry", "name", name, "command", qo.command, "at", time.Now().Format("2006-01-02 15:04:05.000"))
		retryStart := time.Now()
		resp, err = query(ctx, backend, name)
		e.dbg.CommandDone(op+"_retry", retryStart, "error", err, "stats", resp.Stats != nil, "bytes", len(resp.Text))
		if err != nil {
			e.dbg.Log("repeater "+op+" failed", "name", name, "error", err)
			return err
		}
	}
	e.dbg.CommandDone("repeater "+op, cmdStart, "stats", resp.Stats != nil, "bytes", len(resp.Text))
	e.dbg.Log("repeater "+op+" ok", "name", name, "bytes", len(resp.Text), "stats", resp.Stats != nil)
	if resp.Command == "neighbors" {
		var repeater meshcore.Contact
		if ct, err := backend.Contact(ctx, name); err == nil {
			repeater = ct
		}
		if contacts, err := backend.Contacts(ctx); err == nil {
			resp.Neighbours = meshcore.EnrichRepeaterNeighbours(resp.Neighbours, repeater, contacts)
		}
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
		if _, rep, ok := cfg.MatchRepeater(arg); ok {
			return repeaterContactName(arg, rep), start + 1, nil
		}
		if cfg.CurrentRepeater == "" || e.restArg(start+1) != "" {
			return arg, start + 1, nil
		}
	}
	if name := cfg.CurrentRepeaterName(); name != "" {
		return name, start, nil
	}
	if len(cfg.Repeaters) == 1 {
		for _, rep := range cfg.Repeaters {
			return repeaterContactName("", rep), start, nil
		}
	}
	return "", start, fmt.Errorf("no repeater selected; run `mc repeater add <name>` or pass a repeater name")
}

func repeaterContactName(fallback string, rep config.Repeater) string {
	if rep.Name != "" {
		return rep.Name
	}
	return fallback
}

func resolveRepeaterArg(e *env, start int) (string, error) {
	if arg := e.restArg(start); arg != "" {
		return arg, nil
	}
	name, _, err := resolveRepeater(e, start)
	return name, err
}

func loginSavedRepeater(ctx context.Context, e *env, backend Backend, name string, force bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !force {
		if sess, ok := config.CachedRepeaterSession(cfg, name); ok {
			e.dbg.Log("repeater login skipped", "name", sess.Name, "reason", "cached session", "expires_at", sess.ExpiresAt)
			return nil
		}
	}
	ct, err := backend.Contact(ctx, name)
	if err != nil {
		e.dbg.Log("repeater login skipped", "name", name, "reason", "contact not found", "error", err)
		return nil
	}
	rep, ok := cfg.RepeaterByKey(ct.PublicKey)
	if !ok {
		_, rep, ok = cfg.MatchRepeater(name)
	}
	if !ok || rep.Password == "" {
		e.dbg.Log("repeater login skipped", "name", name, "reason", "no saved password")
		return nil
	}
	_, err = ensureRepeaterLogin(ctx, e, backend, ct, rep.Password, force)
	return err
}

func ensureRepeaterLogin(ctx context.Context, e *env, backend Backend, ct meshcore.Contact, password string, force bool) (meshcore.RepeaterSession, error) {
	if force {
		_ = config.RemoveRepeaterSession(ct.PublicKey)
	} else if sess, ok, _ := config.LoadRepeaterSession(ct.PublicKey); ok && sess.Active() {
		e.dbg.Log("repeater login skipped", "name", ct.Name, "reason", "cached session", "expires_at", sess.ExpiresAt)
		return meshcoreSessionFromConfig(sess), nil
	} else {
		hasConnStart := time.Now()
		e.dbg.SendCommand("has_connection", 0x1c, "repeater", ct.Name, "public_key", shortKey(ct.PublicKey))
		active, err := backend.RepeaterHasConnection(ctx, ct.Name)
		e.dbg.CommandDone("has_connection", hasConnStart, "active", active, "error", err)
		switch {
		case err == nil && active:
			e.dbg.Log("repeater login skipped", "name", ct.Name, "reason", "active connection")
			if sess, ok, _ := config.LoadRepeaterSession(ct.PublicKey); ok {
				return meshcoreSessionFromConfig(sess), nil
			}
			return meshcore.RepeaterSession{Repeater: ct.Name, PublicKey: ct.PublicKey}, nil
		case err == nil && !active:
			e.dbg.Log("repeater login", "name", ct.Name, "reason", "no active connection on radio")
		case err != nil:
			e.dbg.Log("repeater has_connection failed", "name", ct.Name, "error", err)
		}
	}
	loginStart := time.Now()
	e.dbg.SendCommand("send_login", 0x1a, "repeater", ct.Name, "public_key", shortKey(ct.PublicKey), "password_len", len(password))
	e.dbg.Log("waiting for login push", "repeater", ct.Name)
	sess, err := backend.RepeaterLogin(ctx, ct.Name, password)
	e.dbg.CommandDone("send_login", loginStart, "error", err)
	if err != nil {
		e.dbg.Log("repeater login failed", "name", ct.Name, "error", err)
		return meshcore.RepeaterSession{}, err
	}
	if err := config.SaveRepeaterSession(configSessionFromMeshcore(sess)); err != nil {
		return sess, err
	}
	return sess, nil
}

func configSessionFromMeshcore(sess meshcore.RepeaterSession) config.RepeaterSession {
	return config.RepeaterSession{
		Name:        sess.Repeater,
		PublicKey:   sess.PublicKey,
		LoggedInAt:  sess.LoggedInAt,
		ExpiresAt:   sess.ExpiresAt,
		Permissions: sess.Permissions,
		Tag:         sess.Tag,
	}
}

func meshcoreSessionFromConfig(sess config.RepeaterSession) meshcore.RepeaterSession {
	return meshcore.RepeaterSession{
		Repeater:    sess.Name,
		PublicKey:   sess.PublicKey,
		LoggedInAt:  sess.LoggedInAt,
		ExpiresAt:   sess.ExpiresAt,
		Permissions: sess.Permissions,
		Tag:         sess.Tag,
	}
}

func printRepeaterResponse(e *env, resp meshcore.RepeaterResponse) error {
	if e.out.JSON {
		if resp.Stats != nil {
			return e.out.JSONValue(map[string]any{
				"repeater": resp.Repeater,
				"command":  resp.Command,
				"received": resp.Received,
				"stats":    resp.Stats.JSONValue(),
			})
		}
		if resp.Command == "neighbors" {
			neighbours := resp.Neighbours
			if neighbours == nil {
				neighbours = []meshcore.RepeaterNeighbour{}
			}
			return e.out.JSONValue(map[string]any{
				"repeater":   resp.Repeater,
				"command":    resp.Command,
				"received":   resp.Received,
				"neighbours": neighbours,
			})
		}
		return e.out.JSONValue(resp)
	}
	if resp.Stats != nil {
		e.out.Human("Repeater: %s\n\n%s\n", resp.Repeater, resp.Stats.Human())
		return nil
	}
	if resp.Command == "neighbors" {
		neighbours := resp.Neighbours
		if neighbours == nil {
			neighbours = []meshcore.RepeaterNeighbour{}
		}
		e.out.Human("%s", meshcore.FormatRepeaterNeighbours(resp.Repeater, neighbours))
		return nil
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
