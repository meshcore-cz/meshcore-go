package ui

import (
	"testing"
	"time"
)

func TestReplicaDetailsFresh(t *testing.T) {
	contactsAt := time.Now().Add(-48 * time.Second)
	channelsAt := time.Now().Add(-2 * time.Minute)
	be := BackendInfo{
		Contacts: ReplicaInfo{Count: 350, SyncedAt: contactsAt},
		Channels: ReplicaInfo{Count: 2, SyncedAt: channelsAt},
	}
	got := replicaDetails(be, Theme{enabled: false})
	want := "350 contacts · 2 channels · updated 48s ago"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReplicaDetailsSyncing(t *testing.T) {
	be := BackendInfo{
		Contacts: ReplicaInfo{
			Syncing:      true,
			SyncReceived: 207,
			SyncTotal:    350,
		},
		Channels: ReplicaInfo{},
	}
	got := replicaDetails(be, Theme{enabled: false})
	want := "contacts replicating 59% (207/350) · channels not replicated"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReplicaDetailsEmpty(t *testing.T) {
	be := BackendInfo{}
	got := replicaDetails(be, Theme{enabled: false})
	want := "contacts not replicated · channels not replicated"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
