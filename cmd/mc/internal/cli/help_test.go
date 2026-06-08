package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Every dispatchable command should be present in the Cobra tree with help text.
func TestEveryCommandHasHelp(t *testing.T) {
	root := NewRoot(&App{})
	commands := []string{
		"connect", "status", "stats", "doctor", "contacts", "contact", "inbox",
		"send", "watch", "trace", "channel", "repeater", "use", "device", "session", "state", "config", "completion", "raw", "version",
	}
	for _, name := range commands {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd == root {
			t.Errorf("missing command %q", name)
			continue
		}
		if cmd.Short == "" {
			t.Errorf("missing short help for command %q", name)
		}
	}
}

func TestCompletionCommands(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{"bash", []string{"# bash completion for", "__start_mc", "complete -o default -F __start_mc mc"}},
		{"zsh", []string{"#compdef mc", "compdef _mc mc", "_mc()"}},
		{"fish", []string{"# fish completion for mc", "complete -c mc", "__mc_perform_completion"}},
	}
	for _, tc := range tests {
		t.Run(tc.shell, func(t *testing.T) {
			root := NewRoot(&App{})
			var buf bytes.Buffer
			root.SetOut(&buf)
			root.SetErr(&buf)
			root.SetArgs([]string{"completion", tc.shell})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("%s completion output missing %q:\n%s", tc.shell, want, out)
				}
			}
		})
	}
}

func TestCompletionHelpIncludesInstallExamples(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{"bash", []string{"source <(mc completion bash)", "mc completion bash > ~/.local/share/bash-completion/completions/mc"}},
		{"zsh", []string{"source <(mc completion zsh)", "mc completion zsh > ~/.zsh/completions/_mc"}},
		{"fish", []string{"mc completion fish | source", "mc completion fish > ~/.config/fish/completions/mc.fish"}},
	}
	for _, tc := range tests {
		t.Run(tc.shell, func(t *testing.T) {
			root := NewRoot(&App{})
			var buf bytes.Buffer
			root.SetOut(&buf)
			cmd, _, err := root.Find([]string{"completion", tc.shell})
			if err != nil {
				t.Fatal(err)
			}
			cmd.SetOut(&buf)
			if err := cmd.Help(); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("%s completion help missing %q:\n%s", tc.shell, want, out)
				}
			}
		})
	}
}

func TestPrintCommandHelpExitCodes(t *testing.T) {
	if code := printCommandHelp("trace"); code != 0 {
		t.Errorf("known command: exit %d, want 0", code)
	}
	if code := printCommandHelp("nope"); code != 2 {
		t.Errorf("unknown command: exit %d, want 2", code)
	}
}

func TestCommandHelpIncludesExamples(t *testing.T) {
	root := NewRoot(&App{})
	var buf bytes.Buffer
	root.SetOut(&buf)
	cmd, _, err := root.Find([]string{"trace"})
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetOut(&buf)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Examples:", "mc trace alice", "mc trace 2525,5153,0455 --return"} {
		if !strings.Contains(out, want) {
			t.Fatalf("trace help missing %q:\n%s", want, out)
		}
	}
}

func TestRawHelpIncludesProtocolReference(t *testing.T) {
	root := NewRoot(&App{})
	var buf bytes.Buffer
	root.SetOut(&buf)
	cmd, _, err := root.Find([]string{"raw"})
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetOut(&buf)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Common command bytes", "0x16  DeviceQuery", "mc raw 16 03"} {
		if !strings.Contains(out, want) {
			t.Fatalf("raw help missing %q:\n%s", want, out)
		}
	}
	shortAt := strings.Index(out, "Send raw bytes directly to the device")
	usageAt := strings.Index(out, "Usage:")
	globalFlagsAt := strings.Index(out, "Global Flags:")
	descriptionAt := strings.Index(out, "Description:")
	detailsAt := strings.Index(out, "Common command bytes")
	examplesAt := strings.Index(out, "Examples:")
	if shortAt < 0 || usageAt < 0 || globalFlagsAt < 0 || descriptionAt < 0 || detailsAt < 0 || examplesAt < 0 {
		t.Fatalf("raw help missing expected sections:\n%s", out)
	}
	if !(shortAt < usageAt && usageAt < globalFlagsAt && globalFlagsAt < descriptionAt && descriptionAt < detailsAt && detailsAt < examplesAt) {
		t.Fatalf("raw help sections in unexpected order:\n%s", out)
	}
}
