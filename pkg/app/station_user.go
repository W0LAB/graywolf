package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	"github.com/chrissnell/graywolf/pkg/webauth"
)

// desktopStationUsername is the account name seeded by ensureStationUser.
// It only matters if the operator later enables cookie login (e.g. the
// desktop app's LAN-access option), which prompts them to set a password.
const desktopStationUsername = "station"

// ensureStationUser creates the single station user when none exists,
// with a random password nobody knows. Used by hosts that authenticate
// the UI with a per-launch bearer (Config.EnsureStationUser): without a
// user, bearer-authed requests reach handlers with no identity, so
// user-scoped endpoints (release notes, AX.25 terminal) return 401 on a
// fresh install.
//
// Idempotent and race-safe: CreateFirstUser refuses when any user
// exists, which is treated as success.
func ensureStationUser(ctx context.Context, auth *webauth.AuthStore, version string, logger *slog.Logger) error {
	users, err := auth.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	if len(users) > 0 {
		return nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate station user secret: %w", err)
	}
	hash, err := webauth.HashPassword(hex.EncodeToString(secret))
	if err != nil {
		return fmt.Errorf("hash station user secret: %w", err)
	}
	if _, err := auth.CreateFirstUser(ctx, desktopStationUsername, hash, version); err != nil {
		if errors.Is(err, webauth.ErrSetupAlreadyComplete) {
			return nil
		}
		return fmt.Errorf("create station user: %w", err)
	}
	logger.Info("created station user for launch-token auth", "username", desktopStationUsername)
	return nil
}
