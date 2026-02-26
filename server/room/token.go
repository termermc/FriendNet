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

	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
)

// The magic number at the beginning of a decrypted token's serialized data.
// Must be incremented everytime the serialization format changes, otherwise
// the deserializer might panic.
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
			for token, invalidUntilTs := range m.expiredTokens {
				if invalidUntilTs.Before(time.Now()) {
					delete(m.expiredTokens, token)
				}
			}
			m.mu.RUnlock()
		}
	}
}

// Token serialization format:
//  - Magic num (1 byte)
//  - Expiration UNIX timestamp (uint64, little endian)
//  - ID (uint64, little endian)
//  - Is server (1 byte)
//  - Target (1 byte len + string content)
//  - Origin (1 byte len + string content)
//  - Room name (1 byte len + string content)

const serMagicNumSize = 1
const serExpSize = 8
const serIdSize = 8
const serIsServerSize = 1

const serMinBufSize = serMagicNumSize + serIdSize + serExpSize + serIsServerSize

// fillBufWithHeader fills a token buffer with its header.
// buf must be at least serMinBufSize bytes long.
func (m *TokenManager) fillBufWithHeader(buf []byte, isServer bool) (offset int) {
	buf[offset] = tokenMagicNum
	offset++

	expTs := time.Now().Add(m.validDuration)
	binary.LittleEndian.PutUint64(buf[offset:offset+serExpSize], uint64(expTs.Unix()))
	offset += serExpSize

	_, _ = rand.Read(buf[offset : offset+serIdSize])
	offset += serIdSize

	if isServer {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset++

	return offset
}

// NewServerToken generates a new server token.
// Server tokens are used by the central server to test client connection methods.
func (m *TokenManager) NewServerToken() string {
	buf := make([]byte, serMinBufSize)

	_ = m.fillBufWithHeader(buf, true)

	nonce := make([]byte, m.gcm.NonceSize())
	_, _ = rand.Read(nonce)

	bytes := m.gcm.Seal(nonce, nonce, buf, nil)
	return base64.RawURLEncoding.EncodeToString(bytes)
}

// NewClientToken generates a new client token.
// Client tokens are generated for clients to use during direct connection handshakes.
func (m *TokenManager) NewClientToken(room common.NormalizedRoomName, origin common.NormalizedUsername, target common.NormalizedUsername) string {
	roomStr := room.String()
	originStr := origin.String()
	targetStr := target.String()

	bufSize := serMinBufSize + 1 + len(roomStr) + 1 + len(originStr) + 1 + len(targetStr)
	buf := make([]byte, bufSize)

	offset := m.fillBufWithHeader(buf, false)
	buf[offset] = uint8(len(targetStr))
	copy(buf[offset+1:], targetStr)
	offset += 1 + len(targetStr)
	buf[offset] = uint8(len(originStr))
	copy(buf[offset+1:], originStr)
	offset += 1 + len(originStr)
	buf[offset] = uint8(len(roomStr))
	copy(buf[offset+1:], roomStr)

	nonce := make([]byte, m.gcm.NonceSize())
	_, _ = rand.Read(nonce)

	bytes := m.gcm.Seal(nonce, nonce, buf, nil)
	return base64.RawURLEncoding.EncodeToString(bytes)
}

// Redeem attempts to redeem a token.
// The result will always be non-nil, and the validity of the token can be checked with the IsValid field.
//
// The token will be invalid if:
func (m *TokenManager) Redeem(redeemer common.NormalizedUsername, token string) *pb.MsgRedeemConnHandshakeTokenResult {
	res := &pb.MsgRedeemConnHandshakeTokenResult{}

	ciphertext, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return res
	}

	nonceSize := m.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return res
	}

	nonce := ciphertext[:nonceSize]
	ciphertext = ciphertext[nonceSize:]

	buf, err := m.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return res
	}

	offset := 0

	if buf[offset] != tokenMagicNum {
		return res
	}
	offset++

	// Check expiration.
	expTs := binary.LittleEndian.Uint64(buf[offset : offset+serExpSize])
	if time.Now().After(time.Unix(int64(expTs), 0)) {
		// Expired.
		return res
	}
	offset += serExpSize

	// Check if redeemed.
	id := binary.LittleEndian.Uint64(buf[offset : offset+serIdSize])
	m.mu.RLock()
	_, isRedeemed := m.expiredTokens[id]
	m.mu.RUnlock()
	if isRedeemed {
		// Already redeemed.
		return res
	}
	offset += serIdSize

	if buf[offset] == 1 {
		res.IsServer = true
		res.IsValid = true

		// Make sure token can't be redeemed twice.
		m.mu.Lock()
		m.expiredTokens[id] = time.Now().Add(m.validDuration)
		m.mu.Unlock()

		return res
	}

	offset++

	targetLen := int(buf[offset])
	target := string(buf[offset+1 : offset+1+targetLen])
	if target != redeemer.String() {
		// Target set by the client that requested the token does not match the redeemer of it.
		return res
	}

	offset += 1 + targetLen

	originLen := int(buf[offset])
	res.Username = string(buf[offset+1 : offset+1+originLen])
	offset += 1 + originLen

	roomLen := int(buf[offset])
	res.Room = string(buf[offset+1 : offset+1+roomLen])

	res.IsValid = true

	// Make sure token can't be redeemed twice.
	m.mu.Lock()
	m.expiredTokens[id] = time.Now().Add(m.validDuration)
	m.mu.Unlock()

	return res
}
