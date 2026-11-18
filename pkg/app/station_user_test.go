package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/chrissnell/graywolf/pkg/configstore"
	"github.com/chrissnell/graywolf/pkg/webauth"
)

func newTestAuth(t *testing.T) *webauth.AuthStore {
	t.Helper()
	store, err := configstore.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	auth, err := webauth.NewAuthStore(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestEnsureStationUser_CreatesOnceOnFreshInstall(t *testing.T) {
	ctx := context.Background()
	auth := newTestAuth(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for i := 0; i < 2; i++ {
		if err := ensureStationUser(ctx, auth, "1.2.3", logger); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	users, err := auth.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("want 1 user, got %d", len(users))
	}
	u := users[0]
	if u.Username != desktopStationUsername {
		t.Errorf("username = %q, want %q", u.Username, desktopStationUsername)
	}
	if u.LastSeenReleaseVersion != "1.2.3" {
		t.Errorf("LastSeenReleaseVersion = %q, want build version", u.LastSeenReleaseVersion)
	}
	if u.PasswordHash == "" {
		t.Error("password hash is empty")
	}
}

func TestEnsureStationUser_LeavesExistingUserAlone(t *testing.T) {
	ctx := context.Background()
	auth := newTestAuth(t)
	hash, err := webauth.HashPassword("hunter2hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateFirstUser(ctx, "w0lab", hash, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := ensureStationUser(ctx, auth, "1.2.3", slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	users, _ := auth.ListUsers(ctx)
	if len(users) != 1 || users[0].Username != "w0lab" || users[0].PasswordHash != hash {
		t.Fatalf("existing user changed: %+v", users)
	}
}
