//go:build !windows && !linux

package main

func InfoBox(title string, content string) {
	// No-op.
}
