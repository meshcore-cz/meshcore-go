package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/reeflective/readline"
	"golang.org/x/term"
)

const (
	shellPromptYellow = "\033[33m"
	shellPromptReset  = "\033[0m"
)

func runShell(ctx context.Context, parent *env, session *shellSession, in io.Reader) error {
	if in != nil {
		return runShellScanner(ctx, parent, session, in)
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return runShellReadline(ctx, parent, session)
	}
	return runShellScanner(ctx, parent, session, os.Stdin)
}

func runShellReadline(ctx context.Context, parent *env, session *shellSession) error {
	rl := readline.NewShell()
	rl.Completer = makeShellCompleter(session)
	rl.Prompt.Primary(func() string {
		return shellPromptText(ctx, session)
	})

	if historyPath, err := shellHistoryPath(); err == nil {
		rl.History.AddFromFile("mc", historyPath)
	}

	stopEvents := startShellEventWatcher(ctx, rl, session)
	defer stopEvents()

	for {
		line, err := rl.Readline()
		switch {
		case errors.Is(err, readline.ErrInterrupt):
			continue
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		}

		if isTerminalNoiseLine(line) {
			continue
		}

		if done := executeShellLine(ctx, parent, session, line, shellReportError); done {
			return nil
		}
	}
}

func runShellScanner(ctx context.Context, parent *env, session *shellSession, in io.Reader) error {
	scanner := bufio.NewScanner(in)
	for {
		parent.out.Human("%s", shellPromptPlain(session))
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			parent.out.Human("\n")
			return nil
		}

		if done := executeShellLine(ctx, parent, session, scanner.Text(), shellReportError); done {
			return nil
		}
	}
}

func executeShellLine(
	ctx context.Context,
	parent *env,
	session *shellSession,
	line string,
	reportError func(format string, args ...any),
) (exit bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}

	fields, err := splitShellFields(line)
	if err != nil {
		reportError("mc: %v", err)
		return false
	}
	if len(fields) == 0 {
		return false
	}

	if fields[0] == "?" {
		fields[0] = "help"
	}
	if shellShouldExit(fields[0]) {
		return true
	}

	args := inheritShellArgs(fields, parent.args)
	cmdCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	err = Execute(cmdCtx, args, ExecuteOptions{
		BackendSocket:         session.Socket,
		RequireIPC:            true,
		InShell:               true,
		TemporaryShellBackend: session.Temporary,
	})
	cancel()

	if err != nil {
		reportError("mc: %v", err)
	}
	return false
}

// shellReportError writes command errors to stderr. Do not use readline's Printf
// here: it assumes an active editing session and emits cursor queries whose
// responses (e.g. ^[[38;19R) leak into the next Readline() call.
func shellReportError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// isTerminalNoiseLine reports lines that are terminal control responses accidentally
// read as input after a cursor query leak.
func isTerminalNoiseLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if strings.HasPrefix(line, "\x1b[") && strings.HasSuffix(line, "R") {
		return true
	}
	if strings.HasPrefix(line, "^[[") && strings.HasSuffix(line, "R") {
		return true
	}
	return false
}

func shellPromptPlain(session *shellSession) string {
	return fmt.Sprintf("mc[%s]> ", session.Profile)
}

func shellPromptText(ctx context.Context, session *shellSession) string {
	label := session.Profile
	if session.Temporary {
		label += shellPromptYellow + ":temp" + shellPromptReset
	} else if degraded, ok := shellBackendDegraded(ctx, session); ok && degraded {
		label += shellPromptYellow + ":degraded" + shellPromptReset
	}
	return fmt.Sprintf("mc[%s]> ", label)
}

func shellBackendDegraded(ctx context.Context, session *shellSession) (bool, bool) {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	st, err := localbackend.NewClient(session.Socket).Status(ctx)
	if err != nil {
		return false, false
	}
	return !st.Healthy, true
}

func startShellEventWatcher(ctx context.Context, rl *readline.Shell, session *shellSession) func() {
	watchCtx, cancel := context.WithCancel(ctx)
	client := localbackend.NewClient(session.Socket)

	events, err := client.Watch(watchCtx)
	if err != nil {
		cancel()
		return func() {}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-watchCtx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if msg, ok := formatShellEvent(ev); ok {
					rl.Printf("%s", msg)
				}
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func formatShellEvent(ev localbackend.Event) (string, bool) {
	switch ev.Type {
	case "message":
		from := ev.From
		if from == "" && ev.Channel != "" {
			from = "#" + ev.Channel
		}
		ts := time.Now().Format("15:04:05")
		if !ev.Timestamp.IsZero() {
			ts = ev.Timestamp.Format("15:04:05")
		}
		return fmt.Sprintf("[%s] %s: %s", ts, from, ev.Text), true
	case "disconnected":
		if ev.Error == "" {
			return "backend disconnected", true
		}
		return "backend disconnected: " + ev.Error, true
	default:
		return "", false
	}
}
