//go:build !linux

package main

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// singleInstanceOptions: a second launch focuses the running window.
func singleInstanceOptions(_ *slog.Logger, onSecondLaunch func()) *application.SingleInstanceOptions {
	return &application.SingleInstanceOptions{
		UniqueID:               singleInstanceID,
		OnSecondInstanceLaunch: func(application.SecondInstanceData) { onSecondLaunch() },
	}
}
