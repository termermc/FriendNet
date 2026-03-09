package appinfo

import "time"

// UpdateCheckerBaseUrl is the base URL for checking for client updates.
const UpdateCheckerBaseUrl = "https://friendnet.org/updater/client"

// UpdateCheckerInterval is the interval at which the client checks for updates.
const UpdateCheckerInterval = 1 * time.Hour
