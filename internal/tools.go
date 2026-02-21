//go:build tools

// Package tools pins build-time and test dependencies so they appear in go.mod.
package tools

import (
	_ "github.com/fsnotify/fsnotify"
	_ "github.com/google/uuid"
	_ "pgregory.net/rapid"
)
