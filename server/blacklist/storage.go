package blacklist

import (
	"context"
	"fmt"
	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/serverrpc/v1"
	"friendnet.org/server/storage"
)

// PolicyStorage is an interface that defines methods for storing and retrieving blacklist policies.
type PolicyStorage interface {
	// GetPolicies returns all blacklist policies.
	GetPolicies(ctx context.Context) ([]*pb.BlacklistPolicy, error)

	// AddPolicies adds one or more policies.
	AddPolicies(ctx context.Context, policies []*pb.BlacklistPolicy) error

	// RemovePolicies removes one or more policies by their keywords.
	RemovePolicies(ctx context.Context, keywords []string) error
}

// GlobalStorage can store and retrieve global blacklist policies.
type GlobalStorage struct {
	storage *storage.Storage
}

var _ PolicyStorage = GlobalStorage{}

// NewGlobalStorage creates a new GlobalStorage.
func NewGlobalStorage(storage *storage.Storage) GlobalStorage {
	return GlobalStorage{
		storage: storage,
	}
}

func (s GlobalStorage) GetPolicies(ctx context.Context) ([]*pb.BlacklistPolicy, error) {
	policies, err := s.storage.GetBlacklistPoliciesForRoom(ctx, common.ZeroNormalizedRoomName)
	if err != nil {
		return nil, fmt.Errorf("could not get global blacklist policies: %w", err)
	}

	return policies, nil
}

func (s GlobalStorage) AddPolicies(ctx context.Context, policies []*pb.BlacklistPolicy) error {
	err := s.storage.AddPoliciesToBlacklist(ctx, common.ZeroNormalizedRoomName, policies)
	if err != nil {
		return fmt.Errorf(`failed to add global blacklist policies: %w`, err)
	}
	return nil
}

func (s GlobalStorage) RemovePolicies(ctx context.Context, keywords []string) error {
	err := s.storage.RemovePoliciesFromBlacklist(ctx, common.ZeroNormalizedRoomName, keywords)
	if err != nil {
		return fmt.Errorf(`failed to remove policies from global blacklist: %w`, err)
	}
	return nil
}

// RoomStorage can store and retrieve room-specific blacklist policies.
type RoomStorage struct {
	storage *storage.Storage
	room    common.NormalizedRoomName
}

var _ PolicyStorage = (*RoomStorage)(nil)

// NewRoomStorage creates a new RoomStorage.
func NewRoomStorage(storage *storage.Storage, room common.NormalizedRoomName) *RoomStorage {
	return &RoomStorage{
		storage: storage,
		room:    room,
	}
}

func (s RoomStorage) GetPolicies(ctx context.Context) ([]*pb.BlacklistPolicy, error) {
	policies, err := s.storage.GetBlacklistPoliciesForRoom(ctx, s.room)
	if err != nil {
		return nil, fmt.Errorf("could not get room %q blacklist policies: %w", s.room.String(), err)
	}

	return policies, nil
}

func (s RoomStorage) AddPolicies(ctx context.Context, policies []*pb.BlacklistPolicy) error {
	err := s.storage.AddPoliciesToBlacklist(ctx, s.room, policies)
	if err != nil {
		return fmt.Errorf(`failed to add room %q blacklist policies: %w`, s.room.String(), err)
	}
	return nil
}

func (s RoomStorage) RemovePolicies(ctx context.Context, keywords []string) error {
	err := s.storage.RemovePoliciesFromBlacklist(ctx, s.room, keywords)
	if err != nil {
		return fmt.Errorf(`failed to remove policies from room %q blacklist: %w`, s.room.String(), err)
	}
	return nil
}

// MemoryStorage can store and retrieve blacklist policies in memory.
// Meant for testing, not threadsafe.
type MemoryStorage struct {
	policies map[string]*pb.BlacklistPolicy
}

var _ PolicyStorage = MemoryStorage{}

// NewMemoryStorage creates a new MemoryStorage.
func NewMemoryStorage(storage *storage.Storage) MemoryStorage {
	return MemoryStorage{
		policies: make(map[string]*pb.BlacklistPolicy),
	}
}

func (s MemoryStorage) GetPolicies(_ context.Context) ([]*pb.BlacklistPolicy, error) {
	res := make([]*pb.BlacklistPolicy, 0, len(s.policies))
	for _, policy := range s.policies {
		res = append(res, policy)
	}
	return res, nil
}

func (s MemoryStorage) AddPolicies(_ context.Context, policies []*pb.BlacklistPolicy) error {
	for _, policy := range policies {
		s.policies[policy.Keyword] = policy
	}
	return nil
}

func (s MemoryStorage) RemovePolicies(_ context.Context, keywords []string) error {
	for _, key := range keywords {
		s.policies[key] = nil
	}
	return nil
}
