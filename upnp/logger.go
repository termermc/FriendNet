// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package upnp

import (
	"fmt"
	"log/slog"
)

var logger = slog.Default()

// SetLogger configures the package logger. Passing nil resets to slog.Default().
func SetLogger(l *slog.Logger) {
	if l == nil {
		logger = slog.Default()
		return
	}
	logger = l
}

func debugln(args ...any) {
	if logger == nil {
		return
	}
	logger.Debug(fmt.Sprint(args...))
}

func debugf(format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Debug(fmt.Sprintf(format, args...))
}
