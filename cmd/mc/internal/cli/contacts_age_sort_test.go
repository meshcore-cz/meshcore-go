package cli

import (
	"testing"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func TestSortAgeOrder(t *testing.T) {
	now := time.Now()
	contacts := []meshcore.Contact{
		{Name: "a-1h", LastAdvert: now.Add(-90 * time.Minute)},
		{Name: "b-2m", LastAdvert: now.Add(-2 * time.Minute)},
		{Name: "c-2h", LastAdvert: now.Add(-150 * time.Minute)},
		{Name: "d-3h", LastAdvert: now.Add(-200 * time.Minute)},
		{Name: "e-skew2h", LastAdvert: now.Add(2 * time.Hour)},
	}
	got, err := filterContacts(contacts, contactListQuery{sortBy: "age"}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"d-3h", "c-2h", "a-1h", "b-2m", "e-skew2h"}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("index %d = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestSortAgePutsNeverLast(t *testing.T) {
	now := time.Now()
	contacts := []meshcore.Contact{
		{Name: "never", LastAdvert: time.Time{}},
		{Name: "recent", LastAdvert: now.Add(-5 * time.Minute)},
		{Name: "old", LastAdvert: now.Add(-3 * time.Hour)},
		{Name: "skew", LastAdvert: now.Add(time.Hour)},
	}
	got, err := filterContacts(contacts, contactListQuery{sortBy: "age"}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"old", "recent", "skew", "never"}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("index %d = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestSortAgeIsPortable(t *testing.T) {
	fixed := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	contacts := []meshcore.Contact{
		{Name: "older", LastAdvert: fixed.Add(-3 * time.Hour)},
		{Name: "newer", LastAdvert: fixed.Add(-30 * time.Minute)},
		{Name: "skew", LastAdvert: fixed.Add(2 * time.Hour)},
	}
	order := func(now time.Time) []string {
		orig := timeNow
		timeNow = func() time.Time { return now }
		defer func() { timeNow = orig }()

		got, err := filterContacts(append([]meshcore.Contact(nil), contacts...), contactListQuery{sortBy: "age"}, 0, 0)
		if err != nil {
			t.Fatalf("filter: %v", err)
		}
		return names(got)
	}

	a := order(fixed)
	b := order(fixed.Add(30 * time.Minute))
	if len(a) != len(b) {
		t.Fatalf("len mismatch: %v vs %v", a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("order changed with local clock shift: %v vs %v", a, b)
		}
	}
}

func names(contacts []meshcore.Contact) []string {
	out := make([]string, len(contacts))
	for i, c := range contacts {
		out[i] = c.Name
	}
	return out
}
