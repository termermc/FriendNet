package room

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"friendnet.org/ahocorasick"
	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/serverrpc/v1"
	"friendnet.org/server/storage"
)

// Blacklist stores blocked keywords.
// This object maintains a string matching engine through a persistent table in storage.
// On server start, it will fetch and build the engine from said storage.
// This object can also be dynamically updated to add/remove words to/from the engine.
type Blacklist struct {
	mu sync.RWMutex

	ctx     context.Context
	room    common.NormalizedRoomName
	storage *storage.Storage
	machine *ahocorasick.Machine

	wholeWords     map[string]struct{}
	hasAnyKeywords bool
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

		wholeWords:     make(map[string]struct{}),
		hasAnyKeywords: false,
	}

	err := blacklist.UpdateFromDb()
	if err != nil {
		return nil, fmt.Errorf("UpdateFromDb failed: %w", err)
	}

	return blacklist, nil
}

// UpdateFromDb will update the string matching engine with all matching policies.
func (b *Blacklist) UpdateFromDb() error {
	policies, err := b.storage.GetBlacklistPoliciesForRoom(b.ctx, b.room)
	if err != nil {
		return fmt.Errorf("could not get blacklisted policies: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Set it to false until we know the build succeeded.
	b.hasAnyKeywords = false

	if len(policies) == 0 {
		return nil
	}

	keywords := make([][]rune, 0, len(policies))
	clear(b.wholeWords)

	for _, policy := range policies {
		keyword := common.ToLowerUnicode(policy.Keyword)

		keywords = append(keywords, []rune(keyword))

		if policy.Mode == pb.BlacklistMatchMode_BLACKLIST_MATCH_MODE_WHOLE {
			b.wholeWords[keyword] = struct{}{}
		}
	}

	err = b.machine.Build(keywords)
	if err != nil {
		return err
	}

	b.hasAnyKeywords = true

	return nil
}

// ErrEmptyKeyword is returned when trying to create a policy with an empty keyword.
var ErrEmptyKeyword = errors.New("tried to add blacklist policy with empty keyword")

// AddPolicies will add blacklist policies to the database and then update the string matching engine.
func (b *Blacklist) AddPolicies(policies []*pb.BlacklistPolicy) error {
	for _, policy := range policies {
		if policy.Keyword == "" {
			return ErrEmptyKeyword
		}
	}

	err := b.storage.AddPoliciesToBlacklist(b.ctx, b.room, policies)
	if err != nil {
		return err
	}

	return b.UpdateFromDb()
}

// Remove will remove keywords from the database and then update the string matching engine.
func (b *Blacklist) Remove(keywords []string) error {
	err := b.storage.RemovePoliciesFromBlacklist(b.ctx, b.room, keywords)
	if err != nil {
		return err
	}

	return b.UpdateFromDb()
}

// Match runs a multi-pattern search on a given string and returns true if there is a match of any kind.
// Haystack should be in lowercase.
func (b *Blacklist) Match(haystack []rune) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.hasAnyKeywords {
		return false
	}

	matched := false
	results := b.machine.MultiPatternSearch(haystack, true)

	// Check for whole words
	for _, result := range results {
		word := string(result.Word)
		if _, has := b.wholeWords[word]; has {
			wordEnd := result.Pos + len(word)
			if wordEnd > len(haystack) || wordEnd < 0 {
				continue
			}

			// If end of word token a-z0-9 + unicode then it is a delimiter and we consider it a match
			wordEndChar := haystack[wordEnd]
			// Checking if the rune is more than 255 tells us whether it's unicode
			if (wordEndChar >= 'a' && wordEndChar <= 'z') || (wordEndChar >= '0' && wordEndChar <= '9') || wordEndChar > 255 {
				matched = true
				break
			}
		} else {
			matched = true
			break
		}
	}

	return matched
}
