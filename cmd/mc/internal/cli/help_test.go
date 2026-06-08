package cli

import "testing"

// Every dispatchable command should have a help entry.
func TestEveryCommandHasHelp(t *testing.T) {
	commands := []string{
		"connect", "status", "stats", "doctor", "contacts", "contact", "inbox",
		"send", "watch", "trace", "channel", "repeater", "use", "device", "session", "state", "config", "raw", "version",
	}
	for _, cmd := range commands {
		if _, ok := commandHelp[cmd]; !ok {
			t.Errorf("missing help for command %q", cmd)
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
