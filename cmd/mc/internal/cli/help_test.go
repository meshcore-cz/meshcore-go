package cli

import "testing"

// Every dispatchable command should be present in the Cobra tree with help text.
func TestEveryCommandHasHelp(t *testing.T) {
	root := NewRoot(&App{})
	commands := []string{
		"connect", "status", "stats", "doctor", "contacts", "contact", "inbox",
		"send", "watch", "trace", "channel", "repeater", "use", "device", "session", "state", "config", "raw", "version",
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

func TestPrintCommandHelpExitCodes(t *testing.T) {
	if code := printCommandHelp("trace"); code != 0 {
		t.Errorf("known command: exit %d, want 0", code)
	}
	if code := printCommandHelp("nope"); code != 2 {
		t.Errorf("unknown command: exit %d, want 2", code)
	}
}
