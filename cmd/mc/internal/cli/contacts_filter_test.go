package cli

import (
	"math"
	"testing"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/protocol/pathhash"
)

func TestParseContactTypeFilter(t *testing.T) {
	tests := []struct {
		in   string
		want meshcore.ContactType
	}{
		{"repeater", meshcore.ContactRepeater},
		{"companion", meshcore.ContactChat},
		{"chat", meshcore.ContactChat},
		{"room", meshcore.ContactRoom},
	}
	for _, tc := range tests {
		got, err := parseContactTypeFilter(tc.in)
		if err != nil {
			t.Fatalf("parseContactTypeFilter(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseContactTypeFilter(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseWithinKM(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"10km", 10},
		{"4.1km", 4.1},
		{"500m", 0.5},
	}
	for _, tc := range tests {
		got, err := parseWithinKM(tc.in)
		if err != nil {
			t.Fatalf("parseWithinKM(%q): %v", tc.in, err)
		}
		if math.Abs(got-tc.want) > 0.001 {
			t.Fatalf("parseWithinKM(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestFilterContacts(t *testing.T) {
	originLat, originLon := 50.0, 14.0
	old := time.Now().Add(-24 * time.Hour)
	older := time.Now().Add(-48 * time.Hour)
	contacts := []meshcore.Contact{
		{Name: "near-repeater", Type: meshcore.ContactRepeater, Latitude: 50.04, Longitude: 14.0, OutPathEnc: 0x01, OutPath: []byte{0xa9}, LastAdvert: old},
		{Name: "far-repeater", Type: meshcore.ContactRepeater, Latitude: 51.0, Longitude: 14.0, OutPathEnc: pathhash.OutPathUnknown, LastAdvert: older},
		{Name: "near-room", Type: meshcore.ContactRoom, Latitude: 50.02, Longitude: 14.0, OutPathEnc: 0x40, LastAdvert: time.Now()},
		{Name: "no-coords", Type: meshcore.ContactRepeater, PublicKey: "bbbb"},
		{Name: "alpha", Type: meshcore.ContactChat, PublicKey: "aaaa"},
	}

	q := contactListQuery{typeFilter: meshcore.ContactRepeater, hasType: true, sortBy: defaultContactSort}
	got, err := filterContacts(contacts, q, originLat, originLon)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("type filter len = %d, want 3", len(got))
	}

	q = contactListQuery{withinKM: 5, hasWithin: true, sortBy: defaultContactSort}
	got, err = filterContacts(contacts, q, originLat, originLon)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("within filter len = %d, want 2", len(got))
	}

	q = contactListQuery{sortBy: "distance"}
	got, err = filterContacts(contacts, q, originLat, originLon)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "near-room" {
		t.Fatalf("sort distance first = %q, want near-room", got[0].Name)
	}
	if got[len(got)-1].Name != "no-coords" {
		t.Fatalf("sort distance last = %q, want no-coords", got[len(got)-1].Name)
	}

	q = contactListQuery{sortBy: "name"}
	got, err = filterContacts(contacts, q, originLat, originLon)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "alpha" {
		t.Fatalf("sort name first = %q, want alpha", got[0].Name)
	}

	q = contactListQuery{sortBy: "type"}
	got, err = filterContacts(contacts, q, originLat, originLon)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "alpha" {
		t.Fatalf("sort type first = %q, want alpha (companion)", got[0].Name)
	}

	q = contactListQuery{sortBy: "age"}
	got, err = filterContacts(contacts, q, originLat, originLon)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "far-repeater" {
		t.Fatalf("sort age first = %q, want far-repeater", got[0].Name)
	}

	q = contactListQuery{sortBy: "route"}
	got, err = filterContacts(contacts, q, originLat, originLon)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "alpha" {
		t.Fatalf("sort route first = %q, want alpha (direct)", got[0].Name)
	}

	q = contactListQuery{sortBy: "key"}
	got, err = filterContacts(contacts, q, originLat, originLon)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].PublicKey != "aaaa" {
		t.Fatalf("sort key first = %q, want aaaa", got[0].PublicKey)
	}
}
