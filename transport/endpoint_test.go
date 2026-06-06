package transport

import "testing"

func TestParseURI(t *testing.T) {
	tests := []struct {
		in      string
		scheme  string
		address string
		wantErr bool
	}{
		{"serial:///dev/ttyACM0", "serial", "/dev/ttyACM0", false},
		{"ble://C4:20:12:34:56:78", "ble", "C4:20:12:34:56:78", false},
		{"tcp://192.168.1.20:5000", "tcp", "192.168.1.20:5000", false},
		{"/dev/ttyACM0", "", "", true},
		{"://nope", "", "", true},
	}
	for _, tt := range tests {
		u, err := ParseURI(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseURI(%q): expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseURI(%q): unexpected error %v", tt.in, err)
			continue
		}
		if u.Scheme != tt.scheme {
			t.Errorf("ParseURI(%q): scheme = %q, want %q", tt.in, u.Scheme, tt.scheme)
		}
		if got := Address(u); got != tt.address {
			t.Errorf("Address(%q) = %q, want %q", tt.in, got, tt.address)
		}
	}
}
