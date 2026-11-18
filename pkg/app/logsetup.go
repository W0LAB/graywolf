package app

import (
	"context"
	"log/slog"

	"github.com/chrissnell/graywolf/pkg/configstore"
	"github.com/chrissnell/graywolf/pkg/logbuffer"
)

// SetupLogger wraps inner with a logbuffer.Handler that persists records
// to a standalone SQLite ring. Path selection is environment-aware
// (Pi/SD-card -> ramdisk; everywhere else -> next to graywolf.db). If
// the ring DB cannot be opened we log a warning through the inner
// handler and return it unwrapped -- graywolf must boot even when the
// ring is unavailable.
//
// Shared by cmd/graywolf and cmd/graywolf-desktop; the caller stores the
// returned DB in Config.LogBuffer so /api/system-logs can read it.
//
// The configstore override (logbuffer_configs.max_rows) is read here
// best-effort: a missing or unreadable configstore yields the
// environment default. The override takes effect on the next graywolf
// start; live reload is intentionally out of scope.
func SetupLogger(inner slog.Handler, cfg Config) (*slog.Logger, *logbuffer.DB) {
	const (
		ringSizeRamdisk     = 2000
		ringSizeDisk        = 5000
		maintenanceInterval = 200
	)
	// Single fallback logger reused for every pre-SetDefault WARN line
	// in this function, instead of constructing one per call.
	fallback := slog.New(inner)

	var (
		isPi     = logbuffer.IsRaspberryPiHost()
		dbDev, _ = logbuffer.BackingDeviceFor(cfg.DBPath)
		isSD     = logbuffer.IsSDCardDevice(dbDev)
	)
	target, err := logbuffer.ResolvePath(logbuffer.ResolveOptions{
		ConfigDBPath:    cfg.DBPath,
		PreferRamdisk:   cfg.LogBufferRamdisk,
		IsRaspberryPi:   isPi,
		BackingIsSDCard: isSD,
	})
	if err != nil {
		fallback.Warn("logbuffer: path resolution failed; falling back to console-only", "err", err)
		return fallback, nil
	}

	db, err := logbuffer.Open(target)
	if err != nil {
		fallback.Warn("logbuffer: open failed; falling back to console-only", "err", err, "path", target)
		return fallback, nil
	}

	wantedRamdisk := isPi || isSD || cfg.LogBufferRamdisk
	if wantedRamdisk && !logbuffer.IsRamdiskPath(target) {
		// Spec § Subsystem 1: "Fall back to disk with a WARN log if no
		// ramdisk is writable." Surface this through the inner handler
		// since SetDefault hasn't run yet.
		fallback.Warn("logbuffer: no ramdisk writable; falling back to disk-backed ring", "path", target)
	}

	ringSize := ringSizeDisk
	if wantedRamdisk {
		ringSize = ringSizeRamdisk
	}
	override, hasOverride := readMaxRowsOverride(cfg.DBPath, fallback)
	if hasOverride {
		if override <= 0 {
			// Operator explicitly disabled persistence.
			_ = db.Close()
			fallback.Warn("logbuffer: persistence disabled by configstore (logbuffer.max_rows=0)")
			return fallback, nil
		}
		ringSize = override
	}

	h := logbuffer.New(inner, db, logbuffer.Config{
		RingSize:         ringSize,
		MaintenanceEvery: maintenanceInterval,
	})
	logger := slog.New(h)
	logger.Info("logbuffer: persistence enabled", "path", target, "ring_size", ringSize)
	return logger, db
}

// readMaxRowsOverride opens the configstore (read-only intent) and
// returns the operator's max_rows override along with a "has-override"
// flag. The flag is critical because MaxRows == 0 with hasOverride
// means "operator opted out of persistence" -- distinct from "no
// override stored, use environment default".
//
// Errors fall back to "no override" so a misconfigured configstore
// does not prevent graywolf from booting; the rest of the program
// will fail with a clearer error when it tries to use the configstore
// for real. Genuine errors (distinct from "row not present", which
// GetLogBufferConfig converts to exists=false, err=nil) are surfaced
// here as a WARN so the operator gets an early signal.
//
// Cost note: this is the first of two configstore.Open calls during
// startup (the second is in app.wiring). Both run Migrate(), but
// migrations are gated by PRAGMA user_version so the second pass is a
// no-op on the schema; the AutoMigrate sweep + idempotent seeds still
// run twice, which is wasteful but small. If startup latency ever
// matters, expose a configstore read-only entry point that skips
// Migrate, or thread the override through app.New.
func readMaxRowsOverride(dbPath string, fallback *slog.Logger) (int, bool) {
	store, err := configstore.Open(dbPath)
	if err != nil {
		fallback.Warn("logbuffer: configstore open for max_rows override failed; using environment default", "err", err)
		return 0, false
	}
	defer store.Close()
	cfg, exists, err := store.GetLogBufferConfig(context.Background())
	if err != nil {
		fallback.Warn("logbuffer: configstore read for max_rows override failed; using environment default", "err", err)
		return 0, false
	}
	if !exists {
		return 0, false
	}
	return cfg.MaxRows, true
}
