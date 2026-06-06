package cli

import (
	"reflect"
	"testing"
)

func TestSplitShellFields(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "plain",
			in:   "status",
			want: []string{"status"},
		},
		{
			name: "quoted message",
			in:   `send alice "hello there" --wait`,
			want: []string{"send", "alice", "hello there", "--wait"},
		},
		{
			name: "single quoted channel",
			in:   `channel send Public 'hello mesh'`,
			want: []string{"channel", "send", "Public", "hello mesh"},
		},
		{
			name: "escaped space",
			in:   `send alice hello\ there`,
			want: []string{"send", "alice", "hello there"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitShellFields(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitShellFields(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitShellFieldsErrors(t *testing.T) {
	for _, in := range []string{`send alice "unterminated`, `send alice trailing\`} {
		if _, err := splitShellFields(in); err == nil {
			t.Fatalf("splitShellFields(%q) succeeded, want error", in)
		}
	}
}
