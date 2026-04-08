package updater

import "time"

// UpdateCheckerBaseUrl is the base URL for checking for updates.
// Currently, the client and server are versioned together, so they use the same base URL.
const UpdateCheckerBaseUrl = "https://friendnet.org/updater/client"

// UpdateCheckerInterval is the interval at which the program checks for updates.
const UpdateCheckerInterval = 1 * time.Hour
