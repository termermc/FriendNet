//go:generate npm ci
//go:generate npm run build

package webui

import "embed"

// Dist contains the embedded web UI files.
//
//go:embed dist/*
var Dist embed.FS

// TODO Add functions to launch a webserver that serves the web UI.
// TODO Ensure CSP headers are set that prevent any inline or external anything.

// TODO On the future client RPC, ensure the CORS is set to prevent websites from interacting with the RPC.

// TODO On the future client RPC, require an authorization header that's generated if not already in the client database (with a flag to regenerate it).
// This token will be included in the web UI URL that is opened in the browser and will be consumed by the webapp during startup to know how to authenticate.
// During startup, the RPC URL should be included in the web UI URL as well, so that it knows how to reach it.
