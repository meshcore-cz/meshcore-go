package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
)

type shellSession struct {
	Profile   string
	Socket    string
	Temporary bool

	daemon     *localbackend.Daemon
	done       chan error
	cleanup    func()
	completion *shellCompletionCache
}

func (s *shellSession) close() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

func cmdShell(ctx context.Context, e *env) error {
	session, err := openShellSession(ctx, e)
	if err != nil {
		return err
	}
	defer func() {
		if session.Temporary {
			e.out.Human("Stopping temporary backend...\n")
		}
		session.close()
	}()

	printShellBanner(e, session)
	e.out.Human("Type `help` for commands, `exit` to quit.\n\n")
	return runShell(ctx, e, session, nil)
}

func printShellBanner(e *env, session *shellSession) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := localbackend.NewClient(session.Socket)
	st, err := client.Status(ctx)
	if err != nil {
		if session.Temporary {
			e.out.Human("Starting temporary backend for %s...\n", session.Profile)
		}
		return
	}

	if session.Temporary {
		e.out.Human("Starting temporary backend for %s...\n", session.Profile)
		label := shellConnectedLabel(st)
		transport := strings.ToUpper(st.Transport)
		if transport == "" {
			transport = strings.ToUpper(schemeOf(st.URI))
		}
		e.out.Human("Connected to %s via %s.\n", label, transport)
		e.out.Human("Temporary backend will stop when this shell exits.\n")
		return
	}

	e.out.Human("Using backend for %s (pid %d).\n", session.Profile, st.PID)
}

func shellConnectedLabel(st localbackend.Status) string {
	if st.Device.PublicKey != "" {
		return shortKey(st.Device.PublicKey)
	}
	if st.Device.Name != "" {
		return st.Device.Name
	}
	return st.URI
}

func openShellSession(ctx context.Context, e *env) (*shellSession, error) {
	profile, uri, err := resolveShellTarget(e)
	if err != nil {
		return nil, err
	}
	if profile == "" {
		profile = "default"
	}

	socket := resolveBackendSocket(e)
	client := localbackend.NewClient(socket)
	if st, err := client.Status(ctx); err == nil && st.Healthy {
		return &shellSession{
			Profile:   profile,
			Socket:    socket,
			Temporary: false,
		}, nil
	}

	tmpDir, err := os.MkdirTemp("", "mc-shell-*")
	if err != nil {
		return nil, err
	}

	tmpSocket := filepath.Join(tmpDir, "backend.sock")
	opts := append(
		e.dbg.DialOptions(),
		meshcore.WithClientOptions(
			meshcore.WithMessageSync(),
			meshcore.WithTimeout(backendContactSyncTimeout),
		),
	)

	daemon, err := localbackend.NewDaemon(localbackend.DaemonOptions{Socket: tmpSocket})
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	daemon.Register(localbackend.SessionProfile{
		ID:        profile,
		URI:       uri,
		Autostart: true,
		DialOpts:  opts,
	})

	done := make(chan error, 1)
	go func() {
		done <- daemon.Serve()
	}()

	if err := waitForShellBackend(ctx, uri, tmpSocket, done); err != nil {
		daemon.Stop()
		<-done
		os.RemoveAll(tmpDir)
		return nil, err
	}

	return &shellSession{
		Profile:   profile,
		Socket:    tmpSocket,
		Temporary: true,
		daemon:    daemon,
		done:      done,
		cleanup: func() {
			daemon.Stop()
			<-done
			os.RemoveAll(tmpDir)
		},
	}, nil
}

func resolveShellTarget(e *env) (profile, uri string, err error) {
	uri, profile, err = resolveURI(e)
	return profile, uri, err
}

func waitForShellBackend(ctx context.Context, uri, socket string, done <-chan error) error {
	client := localbackend.NewClient(socket)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.NewTimer(backendReadyTimeout(uri))
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			if err != nil {
				return fmt.Errorf("temporary backend failed: %w", err)
			}
			return fmt.Errorf("temporary backend exited before becoming ready")
		case <-timeout.C:
			return fmt.Errorf("temporary backend did not become ready")
		case <-ticker.C:
			st, err := client.Status(ctx)
			if err == nil && st.Healthy {
				return nil
			}
		}
	}
}

func shellShouldExit(cmd string) bool {
	switch cmd {
	case "exit", "quit":
		return true
	default:
		return false
	}
}

func inheritShellArgs(child []string, parent parsedArgs) []string {
	args := append([]string(nil), child...)
	if parent.has("debug") && !containsFlag(args, "--debug") {
		args = append(args, "--debug")
	}
	if parent.has("json") && !containsFlag(args, "--json") {
		args = append(args, "--json")
	}
	return args
}

func containsFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func splitShellFields(s string) ([]string, error) {
	var fields []string
	var b strings.Builder
	var quote rune
	escaped := false
	inField := false

	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
			inField = true
		case r == '\\':
			escaped = true
			inField = true
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			inField = true
		case r == ' ' || r == '\t':
			if inField {
				fields = append(fields, b.String())
				b.Reset()
				inField = false
			}
		default:
			b.WriteRune(r)
			inField = true
		}
	}
	if escaped {
		return nil, fmt.Errorf("unfinished escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if inField {
		fields = append(fields, b.String())
	}
	return fields, nil
}
