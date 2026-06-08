package backend

import (
	"bytes"
	"context"
	"strings"
	"testing"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func TestChannelKeyAndPrivate(t *testing.T) {
	if channelKey(nil) != "" {
		t.Fatal("channelKey(nil) should be empty")
	}
	secret := bytes.Repeat([]byte{0x42}, 16)
	k := channelKey(secret)
	if len(k) != 64 {
		t.Fatalf("channelKey len = %d, want 64 hex chars", len(k))
	}
	if channelKey(secret) != k {
		t.Fatal("channelKey is not stable")
	}

	if isPrivateChannel("anything", nil) {
		t.Fatal("empty secret should not be private")
	}
	// A name-derived key (public/hashtag channel) is not private.
	derived := meshcore.DeriveChannelSecret("#test")
	if isPrivateChannel("#test", derived) {
		t.Fatal("name-derived channel should not be private")
	}
	// A key that does not match the name derivation is private.
	if !isPrivateChannel("#test", secret) {
		t.Fatal("non-name-derived secret should be private")
	}
}

func TestStateStoreChannelsKey(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store, err := OpenStateStore(strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	secret := bytes.Repeat([]byte{0x99}, 16)
	if err := store.UpsertChannels(ctx, []meshcore.Channel{
		{Index: 0, Name: "public"},
		{Index: 1, Name: "rem-ha", Secret: secret},
	}); err != nil {
		t.Fatal(err)
	}

	priv, err := store.Channel(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if priv.Key != channelKey(secret) {
		t.Fatalf("Key = %q, want %q", priv.Key, channelKey(secret))
	}
	if !priv.Private {
		t.Fatal("rem-ha should be private")
	}
	if !bytes.Equal(priv.Channel.Secret, secret) {
		t.Fatal("secret not round-tripped")
	}

	// A channel with no secret has no universal key.
	pub, err := store.Channel(ctx, "public")
	if err != nil {
		t.Fatal(err)
	}
	if pub.Key != "" || pub.Private {
		t.Fatalf("public channel = %+v, want empty key and not private", pub)
	}
}
