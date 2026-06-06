package cli

import "testing"

func TestParseArgs(t *testing.T) {
	pa, err := parseArgs([]string{"--device", "handheld", "status", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if pa.flag("device") != "handheld" {
		t.Errorf("device = %q", pa.flag("device"))
	}
	if !pa.has("json") {
		t.Error("json flag not set")
	}
	if pa.arg(0) != "status" {
		t.Errorf("positional 0 = %q, want status", pa.arg(0))
	}
}

func TestParseArgsEquals(t *testing.T) {
	pa, err := parseArgs([]string{"connect", "--uri=serial:///dev/ttyACM0", "--as=desk"})
	if err != nil {
		t.Fatal(err)
	}
	if pa.flag("uri") != "serial:///dev/ttyACM0" {
		t.Errorf("uri = %q", pa.flag("uri"))
	}
	if pa.flag("as") != "desk" {
		t.Errorf("as = %q", pa.flag("as"))
	}
}

func TestParseArgsMissingValue(t *testing.T) {
	if _, err := parseArgs([]string{"connect", "--uri"}); err == nil {
		t.Error("expected error for value flag without value")
	}
}

func TestParseArgsBooleanAnywhere(t *testing.T) {
	pa, _ := parseArgs([]string{"connect", "--usb", "--no-save"})
	if !pa.has("usb") || !pa.has("no-save") {
		t.Errorf("boolean flags not parsed: %+v", pa.bools)
	}
}
