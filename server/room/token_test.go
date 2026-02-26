package room

import (
	"context"
	"encoding/base64"
	"sync"
	"testing"
	"time"

	"friendnet.org/common"
)

func TestNewTokenManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)
	if tm == nil {
		t.Fatal("expected non-nil TokenManager")
	}
}

func TestNewServerToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	token := tm.NewServerToken()
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Should be valid base64
	if _, err := base64.RawURLEncoding.DecodeString(token); err != nil {
		t.Fatalf("token is not valid base64: %v", err)
	}
}

func TestNewClientToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	room := common.UncheckedCreateNormalizedRoomName("testroom")
	origin := common.UncheckedCreateNormalizedUsername("alice")
	target := common.UncheckedCreateNormalizedUsername("bob")

	token := tm.NewClientToken(room, origin, target)
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	if _, err := base64.RawURLEncoding.DecodeString(token); err != nil {
		t.Fatalf("token is not valid base64: %v", err)
	}
}

func TestRedeemServerToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	token := tm.NewServerToken()
	res := tm.Redeem(common.UncheckedCreateNormalizedUsername("anyone"), token)

	if !res.IsValid {
		t.Fatal("expected valid server token")
	}
	if !res.IsServer {
		t.Fatal("expected IsServer to be true")
	}
}

func TestRedeemClientToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	room := common.UncheckedCreateNormalizedRoomName("testroom")
	origin := common.UncheckedCreateNormalizedUsername("alice")
	target := common.UncheckedCreateNormalizedUsername("bob")

	token := tm.NewClientToken(room, origin, target)
	res := tm.Redeem(target, token)

	if !res.IsValid {
		t.Fatal("expected valid client token")
	}
	if res.IsServer {
		t.Fatal("expected IsServer to be false")
	}
	if res.Username != origin.String() {
		t.Fatalf("expected username %s, got %s",
			origin.String(), res.Username)
	}
	if res.Room != room.String() {
		t.Fatalf("expected room %s, got %s", room.String(), res.Room)
	}
}

func TestRedeemClientTokenWrongTarget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	room := common.UncheckedCreateNormalizedRoomName("testroom")
	origin := common.UncheckedCreateNormalizedUsername("alice")
	target := common.UncheckedCreateNormalizedUsername("bob")

	token := tm.NewClientToken(room, origin, target)
	wrongTarget := common.UncheckedCreateNormalizedUsername("charlie")
	res := tm.Redeem(wrongTarget, token)

	if res.IsValid {
		t.Fatal("expected invalid token when redeemed by wrong target")
	}
}

func TestRedeemTokenTwice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	room := common.UncheckedCreateNormalizedRoomName("testroom")
	origin := common.UncheckedCreateNormalizedUsername("alice")
	target := common.UncheckedCreateNormalizedUsername("bob")

	token := tm.NewClientToken(room, origin, target)

	// First redemption should succeed
	res1 := tm.Redeem(target, token)
	if !res1.IsValid {
		t.Fatal("expected first redemption to be valid")
	}

	// Second redemption should fail
	res2 := tm.Redeem(target, token)
	if res2.IsValid {
		t.Fatal("expected second redemption to be invalid")
	}
}

func TestRedeemExpiredToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 10*time.Millisecond, 10*time.Second)

	token := tm.NewServerToken()

	// Wait for token to expire
	time.Sleep(20 * time.Millisecond)

	res := tm.Redeem(common.UncheckedCreateNormalizedUsername("anyone"), token)
	if res.IsValid {
		t.Fatal("expected expired token to be invalid")
	}
}

func TestRedeemInvalidBase64(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	res := tm.Redeem(common.UncheckedCreateNormalizedUsername("alice"), "not-valid-base64!!!")
	if res.IsValid {
		t.Fatal("expected invalid token")
	}
}

func TestRedeemTamperedToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	token := tm.NewServerToken()

	// Tamper with the token
	decoded, _ := base64.RawURLEncoding.DecodeString(token)
	if len(decoded) > 0 {
		decoded[0] ^= 0xFF // Flip bits
	}
	tamperedToken := base64.RawURLEncoding.EncodeToString(decoded)

	res := tm.Redeem(common.UncheckedCreateNormalizedUsername("alice"), tamperedToken)
	if res.IsValid {
		t.Fatal("expected tampered token to be invalid")
	}
}

func TestRedeemServerTokenMultipleTimes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, DefaultTokenValidDuration, DefaultTokenExpiredGcInterval)

	token := tm.NewServerToken()

	// First redemption should succeed
	res1 := tm.Redeem(common.UncheckedCreateNormalizedUsername("alice"), token)
	if !res1.IsValid {
		t.Fatal("expected first redemption to be valid")
	}

	// Second redemption should fail
	res2 := tm.Redeem(common.UncheckedCreateNormalizedUsername("bob"), token)
	if res2.IsValid {
		t.Fatal("expected second redemption to be invalid")
	}
}

func TestGarbageCollection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	validDuration := 2 * time.Second
	gcInterval := 1 * time.Second
	tm := NewTokenManager(ctx, validDuration, gcInterval)

	room := common.UncheckedCreateNormalizedRoomName("testroom")
	origin := common.UncheckedCreateNormalizedUsername("alice")
	target := common.UncheckedCreateNormalizedUsername("bob")

	token := tm.NewClientToken(room, origin, target)

	// Redeem the token
	res := tm.Redeem(target, token)
	if !res.IsValid {
		t.Fatal("expected valid token")
	}

	tm.mu.RLock()
	initialSize := len(tm.expiredTokens)
	tm.mu.RUnlock()

	if initialSize == 0 {
		t.Fatal("expected expired token to be in map")
	}

	// Wait for token to truly expire and GC to run
	time.Sleep(gcInterval + validDuration + time.Second)

	tm.mu.RLock()
	finalSize := len(tm.expiredTokens)
	tm.mu.RUnlock()

	if finalSize != 0 {
		t.Fatal("expected expired tokens to be garbage collected")
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	// Cancel context
	cancel()

	// Give goroutine time to exit
	time.Sleep(50 * time.Millisecond)

	// TokenManager should still be able to generate tokens
	token := tm.NewServerToken()
	if token == "" {
		t.Fatal("expected token generation to work after context cancel")
	}
}

func TestConcurrentTokenGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	tokens := make(map[string]bool)
	var mu sync.Mutex

	const numGoroutines = 10
	const tokensPerGoroutine = 100

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < tokensPerGoroutine; j++ {
				token := tm.NewServerToken()
				mu.Lock()
				tokens[token] = true
				mu.Unlock()
			}
			done <- true
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	if len(tokens) != numGoroutines*tokensPerGoroutine {
		t.Fatalf("expected %d unique tokens, got %d",
			numGoroutines*tokensPerGoroutine, len(tokens))
	}
}

func TestConcurrentRedemption(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tm := NewTokenManager(ctx, 1*time.Minute, 10*time.Second)

	room := common.UncheckedCreateNormalizedRoomName("testroom")
	origin := common.UncheckedCreateNormalizedUsername("alice")
	target := common.UncheckedCreateNormalizedUsername("bob")

	tokens := make([]string, 10)
	for i := 0; i < 10; i++ {
		tokens[i] = tm.NewClientToken(room, origin, target)
	}

	redeemCount := 0
	var mu sync.Mutex
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			res := tm.Redeem(target, tokens[idx])
			mu.Lock()
			if res.IsValid {
				redeemCount++
			}
			mu.Unlock()
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if redeemCount != 10 {
		t.Fatalf("expected 10 successful redemptions, got %d", redeemCount)
	}
}
