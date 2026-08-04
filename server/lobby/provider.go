package lobby

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os/exec"
	"time"

	"friendnet.org/common"
	"friendnet.org/server/storage"
	mcfpassword "github.com/termermc/go-mcf-password"
)

// AuthStatus are types of authentication response statuses.
type AuthStatus string

const (
	// AuthStatusOk means that the credentials were accepted and the client can join the room.
	AuthStatusOk AuthStatus = "ok"

	// AuthStatusBad means that the credentials were not accepted.
	// If so, the Reason field may contain a specific reason message, or empty to return a generic message.
	AuthStatusBad AuthStatus = "bad"

	// AuthStatusPass means that the provider is satisfied with the credentials, but to pass it to the next provider for
	// actual authentication.
	// Used for middleware.
	AuthStatusPass AuthStatus = "pass"
)

// AuthResponse is a response expected back from an authentication provider.
type AuthResponse struct {
	// The authentication status.
	// See AuthStatus.
	Status AuthStatus `json:"type"`

	// Reason is the reason the authentication was rejected.
	// Only valid for "bad" status.
	Reason string `json:"reason,omitempty"`
}

// AuthProvider provides authentication for users.
type AuthProvider interface {
	// Authenticate authenticates a user to a room.
	// If err is not nil, an actual error happened while trying to authenticate the user.
	Authenticate(
		ctx context.Context,
		ip netip.Addr,
		room common.NormalizedRoomName,
		username common.NormalizedUsername,
		password string,
	) (res AuthResponse, err error)
}

// AccountAuthProvider provides authentication using room accounts.
type AccountAuthProvider struct {
	storage *storage.Storage
}

var _ AuthProvider = (*AccountAuthProvider)(nil)

// NewAccountAuthProvider creates a new AccountAuthProvider using the provided storage.
func NewAccountAuthProvider(storage *storage.Storage) *AccountAuthProvider {
	return &AccountAuthProvider{
		storage: storage,
	}
}

func (p *AccountAuthProvider) Authenticate(
	ctx context.Context,
	_ netip.Addr,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	password string,
) (res AuthResponse, err error) {
	// Look up account and verify password.
	var accountRec storage.AccountRecord
	var hasAcc bool
	accountRec, hasAcc, err = p.storage.GetAccountByRoomAndUsername(ctx, room, username)
	if err != nil {
		return res, err
	}
	if !hasAcc {
		return res, nil
	}

	// Check password.
	var matches bool
	var needsRehash bool
	matches, needsRehash, err = mcfpassword.VerifyPassword(password, accountRec.PasswordHash)
	if err != nil {
		return res, fmt.Errorf(`failed to verify password for account with room %q and username %q: %w`,
			room.String(),
			username.String(),
			err,
		)
	}
	if !matches {
		return AuthResponse{
			Status: AuthStatusBad,
		}, nil
	}

	// Rehash password if necessary.
	if needsRehash {
		var newHash string
		newHash, err = mcfpassword.HashPassword(password)
		if err != nil {
			return res, fmt.Errorf(`failed to rehash password for account with room %q and username %q: %w`,
				room.String(),
				username.String(),
				err,
			)
		}

		err = p.storage.UpdateAccountPasswordHash(ctx, room, username, newHash)
		if err != nil {
			return res, fmt.Errorf(`failed to update account with room %q and username %q with rehashed password: %w`,
				room.String(),
				username.String(),
				err,
			)
		}
	}

	return AuthResponse{
		Status: AuthStatusOk,
	}, nil
}

// ExternalAuthRequest a request sent to an external authentication system.
type ExternalAuthRequest struct {
	// The request type.
	// Should always be "authenticate".
	// Other values are reserved for future use.
	Type string `json:"type"`

	// The client's IP address.
	Ip string `json:"ip"`

	// The room the user is trying to authenticate to.
	Room string `json:"room"`

	// The user's username.
	Username string `json:"username"`

	// The user's password.
	Password string `json:"password"`
}

func mkExternalAuthReq(
	ip netip.Addr,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	password string,
) ([]byte, error) {
	body, err := json.Marshal(ExternalAuthRequest{
		Ip:       ip.Unmap().String(),
		Room:     room.String(),
		Username: username.String(),
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to marshal external auth request body: %w`, err)
	}

	return body, nil
}

// readExternalAuthRes reads an AuthResponse as JSON from the provided io.Reader.
// It validates it after reading it and will return an error if the decoded response is invalid.
func readExternalAuthRes(typ string, r io.Reader) (res AuthResponse, err error) {
	dec := json.NewDecoder(r)

	if err = dec.Decode(&res); err != nil {
		return res, fmt.Errorf(`%s external auth response was not valid JSON: %w`, typ, err)
	}

	// Validate response.
	switch res.Status {
	case AuthStatusOk:
		fallthrough
	case AuthStatusPass:
		if res.Reason != "" {
			return res, fmt.Errorf(`%s external auth returned %q status but included a "reason" value`, typ, res.Reason)
		}
	case AuthStatusBad:
		// Nothing to validate for "bad".
	default:
		if res.Status == "" {
			return res, fmt.Errorf(`%s external auth missing "status" field`, typ)
		}

		return res, fmt.Errorf(`%s external auth returned unknown status: %s`, typ, res.Status)
	}

	return res, nil
}

// HttpAuthProvider provides authentication using an HTTP endpoint.
type HttpAuthProvider struct {
	endpoint string
	client   http.Client
}

// NewHttpAuthProvider creates a new HttpAuthProvider with the specified endpoint and request timeout.
// Returns an error if the endpoint URL is invalid or has an unsupported scheme.
// Supported schemes: "http", "https", "unix".
func NewHttpAuthProvider(endpoint string, timeout time.Duration) (*HttpAuthProvider, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf(`called NewHttpAuthProvider with invalid URL: %w`, err)
	}

	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "unix" {
		return nil, fmt.Errorf(`only http, https and unix URLs are allowed for HTTP external authentication`)
	}

	client := http.Client{
		Timeout: timeout,
	}
	if u.Scheme == "unix" {
		unixPath := u.Opaque + u.Host + u.Path
		client.Transport = &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", unixPath)
			},
		}
	}

	return &HttpAuthProvider{
		endpoint: endpoint,
		client:   client,
	}, nil
}

var _ AuthProvider = (*HttpAuthProvider)(nil)

func (p *HttpAuthProvider) Authenticate(
	ctx context.Context,
	ip netip.Addr,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	password string,
) (res AuthResponse, err error) {
	body, err := mkExternalAuthReq(ip, room, username, password)
	if err != nil {
		return res, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(body))
	if err != nil {
		return res, fmt.Errorf(`failed to create HTTP external auth request: %w`, err)
	}
	req.Header.Add("Content-Type", "application/json")

	httpRes, err := p.client.Do(req)
	if err != nil {
		return res, fmt.Errorf(`HTTP external auth request failed: %w`, err)
	}
	defer func() {
		_ = httpRes.Body.Close()
	}()

	if httpRes.StatusCode != http.StatusOK {
		return res, fmt.Errorf(`HTTP external auth returned status %d %s`, httpRes.StatusCode, httpRes.Status)
	}

	const expectedMime = "application/json"
	contentType := httpRes.Header.Get("Content-Type")
	if contentType == "" {
		return res, fmt.Errorf(`HTTP external auth missing Content-Type header, should be set to %q`, expectedMime)
	}
	if contentType != "application/json" {
		return res, fmt.Errorf(`HTTP external auth Content-Type header value was %q, expected %q`, contentType, expectedMime)
	}

	// Response headers look good; decode and validate request.
	res, err = readExternalAuthRes("HTTP", httpRes.Body)
	if err != nil {
		return res, err
	}

	return res, nil
}

// CmdAuthProvider provides authentication using a shell command or script.
type CmdAuthProvider struct {
	name    string
	args    []string
	timeout time.Duration
}

// NewCmdAuthProvider creates a new CmdAuthProvider with the specified shell command and timeout.
func NewCmdAuthProvider(name string, args []string, timeout time.Duration) (*CmdAuthProvider, error) {
	if name == "" {
		return nil, fmt.Errorf(`called NewCmdAuthProvider with an empty name`)
	}

	return &CmdAuthProvider{
		timeout: timeout,
		name:    name,
		args:    args,
	}, nil
}

var _ AuthProvider = (*CmdAuthProvider)(nil)

func (p *CmdAuthProvider) Authenticate(
	ctx context.Context,
	ip netip.Addr,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	password string,
) (res AuthResponse, err error) {
	body, err := mkExternalAuthReq(ip, room, username, password)
	if err != nil {
		return res, err
	}

	cmd := exec.CommandContext(ctx, p.name, p.args...)
	defer func() {
		_ = cmd.Process.Kill()
	}()

	type cmdRes struct {
		res AuthResponse
		err error
	}
	resChan := make(chan cmdRes, 1)

	go func() {
		r, e := func() (res AuthResponse, err error) {
			stdin, err := cmd.StdinPipe()
			if err != nil {
				return res, fmt.Errorf(`failed to get writer for command external auth stdin: %w`, err)
			}

			var writeIdx int
			var n int
			for writeIdx < len(body) {
				n, err = stdin.Write(body[writeIdx:])
				if err != nil {
					return res, fmt.Errorf(`failed to send request to command external auth: %w`, err)
				}
				writeIdx += n
			}
			_, err = cmd.Stdout.Write([]byte{'\n'})
			if err != nil {
				return res, fmt.Errorf(`failed to send request to command external auth: %w`, err)
			}

			stdout, err := cmd.StdoutPipe()
			if err != nil {
				return res, fmt.Errorf(`failed to get reader for command external auth stdout: %w`, err)
			}

			res, err = readExternalAuthRes("command", stdout)
			if err != nil {
				return res, err
			}

			return res, nil
		}()
		resChan <- cmdRes{r, e}
	}()

	// Respect ctx and timeout.
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, p.timeout)
	defer timeoutCancel()
	select {
	case <-timeoutCtx.Done():
		return res, fmt.Errorf(`command external auth context done before response read: %w`, timeoutCtx.Err())
	case cr := <-resChan:
		return cr.res, cr.err
	}
}
