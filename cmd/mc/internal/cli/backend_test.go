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
		{"device", []string{"status", "--device", "handheld"}, true},
		{"device and direct", []string{"status", "--device", "handheld", "--direct"}, false},
		{"direct", []string{"status", "--direct"}, false},
		{"uri and direct", []string{"raw", "--uri", "serial:///dev/ttyS0", "--direct"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pa := parsedArgs{flags: map[string]string{}, bools: map[string]bool{}}
			for i := 0; i < len(tc.args); i++ {
				switch tc.args[i] {
				case "--uri":
					i++
					pa.flags["uri"] = tc.args[i]
				case "--device":
					i++
					pa.flags["device"] = tc.args[i]
				case "--direct":
					pa.bools["direct"] = true
				}
			}
			if got := preferIPCBackend(&env{args: pa}); got != tc.want {
				t.Fatalf("preferIPCBackend() = %v, want %v", got, tc.want)
			}
		})
	}
}
