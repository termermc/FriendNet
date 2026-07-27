package blacklist

import (
	"context"
	"errors"
	"fmt"
	anyascii "github.com/anyascii/go"
	"sync"

	"friendnet.org/ahocorasick"
	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/serverrpc/v1"
)

// Blacklist stores blocked keywords.
// This object maintains a string matching engine through a persistent table in storage.
// On server start, it will fetch and build the engine from said storage.
// This object can also be dynamically updated to add/remove words to/from the engine.
type Blacklist struct {
	mu sync.RWMutex

	ctx     context.Context
	storage PolicyStorage
	machine *ahocorasick.Machine

	wholeWords     map[string]struct{}
	hasAnyKeywords bool
}

// New creates a new blacklist.
func New(ctx context.Context, storage PolicyStorage) (*Blacklist, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage is nil")
	}

	machine := new(ahocorasick.Machine)

	blacklist := &Blacklist{
		ctx:     ctx,
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
	policies, err := b.storage.GetPolicies(b.ctx)
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

	err := b.storage.AddPolicies(b.ctx, policies)
	if err != nil {
		return err
	}

	return b.UpdateFromDb()
}

// Remove will remove keywords from the database and then update the string matching engine.
func (b *Blacklist) Remove(keywords []string) error {
	err := b.storage.RemovePolicies(b.ctx, keywords)
	if err != nil {
		return err
	}

	return b.UpdateFromDb()
}

func isCharAsciiWord[R rune | byte](r R) bool {
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
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
		wordStr := string(result.Word)

		if _, has := b.wholeWords[wordStr]; has {
			wordStart := result.Pos
			wordEnd := wordStart + len(result.Word) - 1

			// termer 2026/07/27: It's not exactly foolproof, but the idea is this:
			// If a char is ASCII and the next char is an ASCII word char, then it's a substring match.
			// If a char is unicode and the next char is unicode, then it's a substring match.
			// That would allow a string like "blue天" to match full word "blue".
			// It would also allow 藍天 to NOT match full word "天".
			// We also try to detect and convert unicode punctuation to their ASCII counterparts.
			// Things still don't work great for languages without spaces like CJK; substring should be used for those.

			if wordStart > 0 {
				startChar := haystack[wordStart]
				prevChar := haystack[wordStart-1]

				if startChar > 255 && prevChar > 255 {
					// The previous unicode char could be punctuation; test if it is.
					prevAscii := anyascii.TransliterateRune(prevChar)
					if isCharAsciiWord(prevAscii[0]) {
						// Matched a substring
						continue
					}
				} else if startChar <= 255 && isCharAsciiWord(prevChar) {
					// Matched a substring.
					continue
				}
			}

			if wordEnd < len(haystack)-1 {
				endChar := haystack[wordEnd]
				nextChar := haystack[wordEnd+1]

				if endChar > 255 && nextChar > 255 {
					// The next unicode char could be punctuation; test if it is.
					nextAscii := anyascii.TransliterateRune(nextChar)
					if isCharAsciiWord(nextAscii[0]) {
						// Matched a substring
						continue
					}
				} else if endChar <= 255 && isCharAsciiWord(nextChar) {
					// Matched a substring.
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
