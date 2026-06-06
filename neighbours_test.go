package meshcore

import (
	"strings"
	"testing"
)

func TestParseRepeaterNeighbours(t *testing.T) {
	text := "C42066C2:1775:-25\n5EB5233E:28698:-15\nA3923BF5:34463:-13\n51535EFD:66507:29\n"
	got := ParseRepeaterNeighbours(text)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[0].PublicKeyPrefix != "c42066c2" || got[0].HeardSecs != 1775 || got[0].SNRdB != -6.25 {
		t.Fatalf("first = %#v", got[0])
	}
	if got[3].SNRdB != 7.25 {
		t.Fatalf("fourth snr = %v, want 7.25", got[3].SNRdB)
	}
	if ParseRepeaterNeighbours("-none-") != nil {
		t.Fatal("expected nil for -none-")
	}
}

func TestFormatRepeaterNeighbours(t *testing.T) {
	text := "C42066C2:1775:-25\n5EB5233E:28698:-15\n"
	neighbours := ParseRepeaterNeighbours(text)
	out := FormatRepeaterNeighbours("mc.kololec.cz", neighbours)
	for _, want := range []string{
		"Repeater: mc.kololec.cz",
		"2 neighbors:",
		"c42066c2",
		"29m ago",
		"-6.2 dB",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatted output missing %q:\n%s", want, out)
		}
	}
}
