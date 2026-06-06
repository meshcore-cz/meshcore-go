package cli

import "testing"

func TestPreferIPCBackend(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"default", []string{"status"}, true},
		{"uri", []string{"raw", "--uri", "serial:///dev/ttyS0"}, false},
		{"device", []string{"status", "--device", "handheld"}, false},
		{"direct", []string{"status", "--direct"}, false},
		{"uri and direct", []string{"raw", "--uri", "serial:///dev/ttyS0", "--direct"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pa, err := parseArgs(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if got := preferIPCBackend(&env{args: pa}); got != tc.want {
				t.Fatalf("preferIPCBackend() = %v, want %v", got, tc.want)
			}
		})
	}
}
