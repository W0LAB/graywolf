//go:build !android

// graywolf entry point. All runtime wiring lives in pkg/app; this file
// is a thin dispatch shim responsible for build-time version injection,
// subcommand routing, and signal handling. The normal-path main() body
// is app.New(cfg, logger).Run(ctx).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/chrissnell/graywolf/cmd/graywolf/authcli"
	"github.com/chrissnell/graywolf/pkg/app"
	"github.com/chrissnell/graywolf/pkg/logbuffer"
)

// Version and GitCommit are injected at build time via -ldflags. Both
// sides of the build (Go + Rust) format their display string as
// "v<Version>-<GitCommit>"; the Rust side must produce a byte-identical
// string so the startup banner's mismatch check works.
var (
	Version   = "dev"
	GitCommit = "unknown"
)

func fullVersion() string {
	return fmt.Sprintf("v%s-%s", Version, GitCommit)
}

func main() {
	// Pre-flag-parse logger: subcommands (auth, version) only need a
	// minimal handler. The full logbuffer-wrapped handler is constructed
	// after ParseFlags so we know cfg.DBPath and cfg.LogBufferRamdisk.
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if len(os.Args) > 1 {
		// Subcommands are only recognized as os.Args[1]. Keep these cases
		// in sync with app.Subcommands, which ParseFlags uses to detect a
		// misplaced subcommand and hint at the correct argument order.
		switch os.Args[1] {
		case "auth":
			if err := authcli.Run(os.Args[2:], logger, Version); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "flare":
			os.Exit(runFlare(os.Args[2:], Version, GitCommit))
		case "version":
			fmt.Println(fullVersion())
			return
		}
	}

	cfg, err := app.ParseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	cfg.Version = Version
	cfg.GitCommit = GitCommit

	innerLevel := slog.LevelInfo
	if cfg.Debug {
		innerLevel = slog.LevelDebug
	}
	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: innerLevel})

	var logDB *logbuffer.DB
	logger, logDB = app.SetupLogger(inner, cfg)
	slog.SetDefault(logger)
	cfg.LogBuffer = logDB

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.New(cfg, logger).Run(ctx); err != nil {
		logger.Error("graywolf exited with error", "err", err)
		os.Exit(1)
	}
}
