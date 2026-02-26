package room

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

const tokenMagicNum uint8 = 0x90

// DefaultTokenValidDuration is the default duration for which a token is valid.
const DefaultTokenValidDuration = 1 * time.Minute

// DefaultTokenExpiredGcInterval is the default interval for garbage collection of expired tokens.
const DefaultTokenExpiredGcInterval = 10 * time.Second

// TokenManager manages token generation and redemption for direct connections.
// Tokens can only be redeemed by the same TokenManager that created them.
//
// Internally, it only stores the IDs of expired tokens. This makes token generation
// free in terms of memory usage, and expired token IDs are only stored for the
// maximum time a token could be valid, so they do not stay in memory forever.
type TokenManager struct {
	mu       sync.RWMutex
	isClosed bool

	ctx context.Context

	gcm cipher.AEAD

	validDuration     time.Duration
	expiredGcInterval time.Duration

	expiredTokens map[uint64]time.Time
}

// NewTokenManager creates a new TokenManager.
// The TokenManager will stop working once its context is canceled or done.
func NewTokenManager(ctx context.Context, validDuration time.Duration, expiredGcInterval time.Duration) *TokenManager {
	// Set up cipher. We use AES GCM.
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		panic(fmt.Errorf("failed to generate encryption key: %w", err))
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		panic(fmt.Errorf("failed to create AES cipher: %w", err))
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Errorf("failed to create AES GCM: %w", err))
	}

	m := &TokenManager{
		ctx: ctx,

		gcm: gcm,

		validDuration:     validDuration,
		expiredGcInterval: expiredGcInterval,

		expiredTokens: make(map[uint64]time.Time),
	}

	go m.expGc()

	return m
}

func (m *TokenManager) expGc() {
	ticker := time.NewTicker(m.expiredGcInterval)
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			for token, expired := range m.expiredTokens {
				if expired.Before(time.Now()) {
					delete(m.expiredTokens, token)
				}
			}
			m.mu.RUnlock()
		}
	}
}

const serMagicNumSize = 1
const serIdSize = 8
const serExpSize = 8
const serIsServerSize = 1

const serMinBufSize = serMagicNumSize + serIdSize + serExpSize + serIsServerSize

// fillBufWithHeader fills a token buffer with its header.
// buf must be at least serMinBufSize bytes long.
func (m *TokenManager) fillBufWithHeader(buf []byte, isServer bool) (offset int) {
	buf[0] = tokenMagicNum

	_, _ = rand.Read(buf[serMagicNumSize:serIdSize])

	expTs := time.Now().Add(m.validDuration)
	binary.LittleEndian.PutUint64(buf[serMagicNumSize+serIdSize:serMagicNumSize+serIdSize+serExpSize], uint64(expTs.Unix()))

	if isServer {
		buf[serMagicNumSize+serIdSize+serExpSize] = 1
	} else {
		buf[serMagicNumSize+serIdSize+serExpSize] = 0
	}

	return serMinBufSize
}

// NewServerToken generates a new server token.
// Server tokens are used by the central server to test client connection methods.
func (m *TokenManager) NewServerToken() string {
	buf := make([]byte, serMinBufSize)

	_ = m.fillBufWithHeader(buf, true)

	nonce := make([]byte, m.gcm.NonceSize())
	_, _ = rand.Read(nonce)

	bytes := m.gcm.Seal(nonce, nonce, buf, nil)
	return base64.URLEncoding.EncodeToString(bytes)
}

func (m *TokenManager) NewClientToken() string {
	// TODO Calc buf size
	return ""
}
