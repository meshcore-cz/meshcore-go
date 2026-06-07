package ui

import (
	"testing"
)

func TestHasCoordinates(t *testing.T) {
	if HasCoordinates(0, 0) {
		t.Fatal("expected false for 0,0")
	}
	if !HasCoordinates(50.0, 14.0) {
		t.Fatal("expected true for non-zero")
	}
}

func TestDistanceKM(t *testing.T) {
	// Prague ~ Brno, roughly 180–200 km apart.
	pragueLat, pragueLon := 50.0755, 14.4378
	brnoLat, brnoLon := 49.1951, 16.6068
	km := DistanceKM(pragueLat, pragueLon, brnoLat, brnoLon)
	if km < 180 || km > 200 {
		t.Fatalf("Prague-Brno distance = %.1f km, want roughly 180–200 km", km)
	}

	same := DistanceKM(50.1, 14.4, 50.1, 14.4)
	if same > 0.001 {
		t.Fatalf("same point distance = %f, want ~0", same)
	}
}

func TestFormatDistance(t *testing.T) {
	tests := []struct {
		km   float64
		want string
	}{
		{0, "0 m"},
		{-1, "-"},
		{0.74, "740 m"},
		{4.14, "4.1 km"},
		{61.7, "62 km"},
	}
	for _, tc := range tests {
		if got := FormatDistance(tc.km); got != tc.want {
			t.Errorf("FormatDistance(%v) = %q, want %q", tc.km, got, tc.want)
		}
	}
}
