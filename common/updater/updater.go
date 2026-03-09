package updater

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// UpdateInfo stores information about an update.
type UpdateInfo struct {
	// The UNIX timestamp when the update was released.
	CreatedTs int64 `json:"created_ts"`

	// The version string.
	Version string `json:"version"`

	// The update's text description.
	Description string `json:"description"`

	// The URL to the update.
	Url string `json:"url"`
}

func doReq(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("GET %q: %w", url, err)
	}
	req = req.WithContext(ctx)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %q: %w", url, err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %q: server returned status %d %s", url, res.StatusCode, res.Status)
	}

	const maxBodySize = 1024 * 1024
	if res.ContentLength == 0 {
		return nil, fmt.Errorf("GET %q: no content", url)
	}
	if res.ContentLength > maxBodySize {
		return nil, fmt.Errorf("GET %q: response body too large (content-length: %d, max: %d)", url, res.ContentLength, maxBodySize)
	}

	body := make([]byte, res.ContentLength)
	_, err = io.ReadFull(res.Body, body)
	if err != nil {
		return nil, fmt.Errorf("GET %q: failed to read body: %w", url, err)
	}

	return body, nil
}

// ErrInvalidSignature is returned when the signature of the update is invalid.
var ErrInvalidSignature = fmt.Errorf("updater: invalid signature")

// CheckUpdate checks for an update.
//
// The baseUrl string must be the base URL without a trailing slash for the update directory.
// For example: https://friendnet.org/updater/client
//
// If there is an update, it returns the update info.
// If there is no update, it returns nil.
// If the signature for the update is invalid, it returns ErrInvalidSignature.
func CheckUpdate(ctx context.Context, baseUrl string, curUpdate UpdateInfo, pubkey ed25519.PublicKey) (*UpdateInfo, error) {
	// Suffix for cache-busting.
	suffix := fmt.Sprintf("?now=%d", time.Now().Unix())

	updateUrl := baseUrl + "/latest.json" + suffix
	sigUrl := baseUrl + "/latest.json.sig" + suffix

	latestJson, err := doReq(ctx, updateUrl)
	if err != nil {
		return nil, fmt.Errorf(`failed to read latest.json: %w`, err)
	}

	var latest UpdateInfo
	if err = json.Unmarshal(latestJson, &latest); err != nil {
		return nil, fmt.Errorf(`failed to unmarshal latest.json: %w`, err)
	}

	sigB64, err := doReq(ctx, sigUrl)
	if err != nil {
		return nil, fmt.Errorf(`failed to read latest.json.sig: %w`, err)
	}

	sig := make([]byte, (3*len(sigB64)+3)/4)
	n, err := base64.StdEncoding.Decode(sig, sigB64)
	if err != nil {
		return nil, fmt.Errorf(`failed to base64-decode latest.json.sig: %w`, err)
	}
	sig = sig[:n]
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("error: unexpected length of ed25519 signature: expected: %d, got: %d", ed25519.SignatureSize, len(sig))
	}

	ok := ed25519.Verify(pubkey, latestJson, sig)
	if !ok {
		return nil, ErrInvalidSignature
	}

	if latest.CreatedTs <= curUpdate.CreatedTs {
		// Not a new update.
		return nil, nil
	}

	return &latest, nil
}
