package updater

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
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

	var body []byte
	if res.ContentLength == -1 {
		var buf bytes.Buffer
		var n int64
		n, err = io.CopyN(&buf, res.Body, maxBodySize)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("GET %q: failed to read body: %w", url, err)
		}

		if n >= maxBodySize {
			return nil, fmt.Errorf("GET %q: response body too large (stopped reading after %d bytes)", url, n)
		}

		body = buf.Bytes()
	} else {
		body = make([]byte, res.ContentLength)
		_, err = io.ReadFull(res.Body, body)
		if err != nil {
			return nil, fmt.Errorf("GET %q: failed to read body: %w", url, err)
		}
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

// ErrUpdaterClosed is returned when calling a method on a closed updater.
var ErrUpdaterClosed = errors.New("updater is closed")

// UpdateChecker checks for updates at an update base URL.
// It notifies subscribers of new updates.
type UpdateChecker struct {
	mu       sync.Mutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	logger *slog.Logger

	// CurrentUpdate is the current update the client is running.
	// Do not update.
	CurrentUpdate UpdateInfo

	baseUrl  string
	pubkey   ed25519.PublicKey
	interval time.Duration

	newUpdate     *UpdateInfo
	newUpdateErr  error
	newUpdateChan chan struct{}

	// A pretty funny way of doing things.
	// I don't want to have multiple concurrent checks happening at once,
	// so this channel both wakes up the update channel and gives it a channel
	// to close to let us know that it's done.
	checkNowChan chan chan struct{}
}

// NewUpdateChecker creates a new UpdateChecker with the specified data.
//
// See CheckUpdate for more information on the baseUrl, curUpdate, and pubkey parameters.
//
// The interval parameter specifies how often the updater should check for updates.
func NewUpdateChecker(
	logger *slog.Logger,
	baseUrl string,
	curUpdate UpdateInfo,
	pubkey ed25519.PublicKey,
	interval time.Duration,
) *UpdateChecker {
	ctx, ctxCancel := context.WithCancel(context.Background())

	c := &UpdateChecker{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		logger: logger,

		CurrentUpdate: curUpdate,

		baseUrl:  baseUrl,
		pubkey:   pubkey,
		interval: interval,

		newUpdateChan: make(chan struct{}),
		checkNowChan:  make(chan chan struct{}),
	}

	go c.loop()

	return c
}

// Close closes the updater and stops checking for updates.
// Subsequent calls are no-op.
func (c *UpdateChecker) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed {
		return nil
	}

	c.isClosed = true
	c.ctxCancel()

	close(c.newUpdateChan)

	// Sloppy way of preventing close of a closed channel if there is a check in progress that will yield a new update.
	c.newUpdateChan = make(chan struct{})

	return nil
}

func (c *UpdateChecker) loop() {
	ticker := time.NewTimer(c.interval)
	defer ticker.Stop()

	notifyNew := func(info *UpdateInfo, err error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.newUpdate = info
		c.newUpdateErr = err
		close(c.newUpdateChan)
		c.newUpdateChan = make(chan struct{})
	}
	doCheck := func(notifyChan chan struct{}) {
		if notifyChan != nil {
			defer close(notifyChan)
		}

		ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		defer cancel()

		newUpdate, err := CheckUpdate(ctx, c.baseUrl, c.CurrentUpdate, c.pubkey)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) ||
				errors.Is(err, context.Canceled) {
				c.logger.Warn("context deadline exceeded while checking for updates",
					"service", "updater.UpdateChecker",
				)
				return
			}

			if errors.Is(err, ErrInvalidSignature) {
				c.logger.Warn("update signature is invalid",
					"service", "updater.UpdateChecker",
				)
				notifyNew(nil, err)
				return
			}

			c.logger.Error("failed to check for updates",
				"service", "updater.UpdateChecker",
				"err", err,
			)
			return
		}

		notifyNew(newUpdate, nil)
	}

	doCheck(nil)

	for {
		select {
		case <-c.ctx.Done():
			return
		case notifyChan := <-c.checkNowChan:
			doCheck(notifyChan)
		case <-ticker.C:
			doCheck(nil)
		}
	}
}

// GetNewUpdate returns the latest update information available.
// If there is no update, it returns nil.
// If the update was invalid, it returns an error.
// Returns cached data only, call CheckNow to run a check and return the latest information.
func (c *UpdateChecker) GetNewUpdate() (*UpdateInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed {
		return nil, ErrUpdaterClosed
	}

	return c.newUpdate, c.newUpdateErr
}

// CheckNow runs a check and returns the latest update information.
// Checks are run serially, so this function blocks until any pending checks are complete, as well as the scheduled
// check.
// See GetNewUpdate for more information on the returned data.
func (c *UpdateChecker) CheckNow() (*UpdateInfo, error) {
	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return nil, ErrUpdaterClosed
	}
	c.mu.Unlock()

	// Schedule check.
	notifyChan := make(chan struct{})
	c.checkNowChan <- notifyChan

	// Wait for check to finish.
	<-notifyChan

	// Get latest info.
	return c.GetNewUpdate()
}

// NewUpdateChan returns a channel that is closed when there is a new update available.
// The channel is closed immediately when the updater is closed.
func (c *UpdateChecker) NewUpdateChan() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.newUpdateChan
}
