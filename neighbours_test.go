package meshcore

import (
	"math"
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

func TestEnrichRepeaterNeighbours(t *testing.T) {
	neighbours := ParseRepeaterNeighbours("C42066C2:1775:-25\n5EB5233E:28698:-15\n")
	contacts := []Contact{
		{Name: "liba.meshcore.cz", PublicKey: "c42066c2ab12" + strings.Repeat("0", 52), Latitude: 50.0755, Longitude: 14.4378},
	}
	repeater := Contact{Name: "mc.kololec.cz", Latitude: 50.0875, Longitude: 14.4213}
	got := EnrichRepeaterNeighbours(neighbours, repeater, contacts)
	if got[0].Name != "liba.meshcore.cz" {
		t.Fatalf("first name = %q, want liba.meshcore.cz", got[0].Name)
	}
	if got[0].Latitude != 50.0755 || got[0].Longitude != 14.4378 {
		t.Fatalf("first coords = %v,%v", got[0].Latitude, got[0].Longitude)
	}
	if got[0].DistanceKm < 1 || got[0].DistanceKm > 4 {
		t.Fatalf("first distance = %v km, want roughly 2-3 km", got[0].DistanceKm)
	}
	if got[1].Name != "" || got[1].DistanceKm != 0 {
		t.Fatalf("second = %#v, want unresolved", got[1])
	}
}

func TestHaversineDistanceKm(t *testing.T) {
	if got := haversineDistanceKm(50, 14, 50, 14); got != 0 {
		t.Fatalf("same point distance = %v, want 0", got)
	}
	// One degree of latitude is roughly 111 km.
	got := haversineDistanceKm(50, 14, 51, 14)
	if math.Abs(got-111) > 2 {
		t.Fatalf("distance = %.1f km, want roughly 111 km", got)
	}
}

func TestFormatRepeaterNeighbours(t *testing.T) {
	text := "C42066C2:1775:-25\n5EB5233E:28698:-15\n"
	neighbours := ParseRepeaterNeighbours(text)
	neighbours[0].Name = "liba.meshcore.cz"
	neighbours[0].Latitude = 50.0755
	neighbours[0].Longitude = 14.4378
	neighbours[0].DistanceKm = 2.1
	out := FormatRepeaterNeighbours("mc.kololec.cz", neighbours)
	for _, want := range []string{
		"Repeater: mc.kololec.cz",
		"2 neighbors:",
		"PREFIX",
		"NAME",
		"COORDS",
		"DIST",
		"c42066c2",
		"liba.meshcore.cz",
		"50.07550,14.43780",
		"2.1km",
		"29m ago",
		"-6.2 dB",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatted output missing %q:\n%s", want, out)
		}
	}
}
