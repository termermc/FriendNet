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

func isLowerRuneAsciiWord(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9')
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
			wordStart := result.Pos
			wordEnd := wordStart + len(word) - 1

			// termer 2026/07/27: It's not exactly foolproof, but the idea is this:
			// If a char is ASCII and the next char is an ASCII word char, then it's a substring match.
			// If a char is unicode and the next char is unicode, then it's a substring match.
			// That would allow a string like "blue天" to match full word "blue".
			// It would also allow 藍天 to NOT match full word "天".
			// The known limitation here is that （天）(note the unicode full-width parentheses) would NOT match full word "天".
			// I'm mostly worried about languages like Russian where this won't be an issue, but this theoretically could fail on
			// some uses of CJK. In that case, you should probably just use a substring.
			// Some of the above should be solved by using AnyAscii anyway.

			if wordStart > 0 {
				startChar := haystack[wordStart]
				prevChar := haystack[wordStart-1]

				if (startChar > 255 && prevChar > 255) || (startChar <= 255 && isLowerRuneAsciiWord(prevChar)) {
					// Matched a substring
					continue
				}
			}

			if wordEnd < len(haystack)-1 {
				endChar := haystack[wordEnd]
				nextChar := haystack[wordEnd+1]

				if (endChar > 255 && nextChar > 255) || (endChar <= 255 && isLowerRuneAsciiWord(nextChar)) {
					// Matched a substring
					continue
				}
			}

			matched = true
			break
		} else {
			matched = true
			break
		}
	}

	return matched
}
