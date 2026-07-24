package room

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"

	"friendnet.org/ahocorasick"
	"friendnet.org/common"
	"friendnet.org/server/storage"
)

// Blacklist stores blocked keywords.
// This object maintains a string matching engine through a persistent table in storage.
// On server start, it will fetch and build the engine from said storage.
// This object can also be dynamically updated to add/remove words to/from the engine.
type Blacklist struct {
	ctx     context.Context
	room    common.NormalizedRoomName
	storage *storage.Storage
	machine *ahocorasick.Machine
	inUse   atomic.Bool
}

// If room is left empty, this will be treated as a global blacklist policy
func NewBlacklist(ctx context.Context, room common.NormalizedRoomName, storage *storage.Storage) (*Blacklist, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage is nil")
	}

	machine := new(ahocorasick.Machine)

	blacklist := &Blacklist{
		ctx:     ctx,
		room:    room,
		storage: storage,
		machine: machine,
	}

	blacklist.inUse.Store(false)

	err := blacklist.UpdateFromDb()
	if err != nil {
		return nil, fmt.Errorf("UpdateFromDb failed: %w", err)
	}

	return blacklist, nil
}

// UpdateFromDb will update the string matching engine with all matching policies.
func (b *Blacklist) UpdateFromDb() error {
	keywords, err := b.storage.GetBlacklistedKeywordsForRoom(b.ctx, b.room)
	if err != nil {
		return fmt.Errorf("could not get blacklisted keywords: %w", err)
	}

	if len(keywords) == 0 {
		return nil
	}

	b.inUse.Store(true)

	return b.machine.Build(keywords)
}

// Add will add keywords to the database and then update the string matching engine.
func (b *Blacklist) Add(keywords []string) error {
	for _, keyword := range keywords {
		if err := b.storage.AddKeywordToBlacklist(b.ctx, b.room, keyword); err != nil {
			return err
		}
	}

	return b.UpdateFromDb()
}

// Remove will remove keywords from the database and then update the string matching engine.
func (b *Blacklist) Remove(keywords []string) error {
	for _, keyword := range keywords {
		if err := b.storage.RemoveKeywordFromBlacklist(b.ctx, b.room, keyword); err != nil {
			return err
		}
	}

	return b.UpdateFromDb()
}

// Match runs a multi-pattern search on a given string and returns true if there is a match of any kind.
func (b *Blacklist) Match(haystack string) bool {
	if !b.inUse.Load() {
		return false
	}

	runes := bytes.Runes([]byte(haystack))
	results := b.machine.MultiPatternSearch(runes, true)

	return len(results) != 0
}
