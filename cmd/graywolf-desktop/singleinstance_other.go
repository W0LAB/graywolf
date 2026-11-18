//go:build !linux

package main

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func singleInstanceOptions(_ *slog.Logger) *application.SingleInstanceOptions {
	return &application.SingleInstanceOptions{UniqueID: singleInstanceID}
}
