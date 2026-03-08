package updater

import "time"

// UpdateInfo stores information about an update.
type UpdateInfo struct {
	// The timestamp when the update was released.
	Ts time.Time

	// The version string.
	Version string

	// The update's text description.
	Description string

	// The URL to the update.
	Url string
}
