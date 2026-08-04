package blacklist

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"

	anyascii "github.com/anyascii/go"

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

	wholeWords      map[string]struct{}
	hasTrieKeywords bool
	regexes         []*regexp.Regexp
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

		wholeWords:      make(map[string]struct{}),
		hasTrieKeywords: false,
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

	// Set it to false until we know the trie build succeeded.
	b.hasTrieKeywords = false

	trieKeywords := make([][]rune, 0, len(policies))
	clear(b.wholeWords)
	b.regexes = b.regexes[0:0]

	if len(policies) == 0 {
		return nil
	}

	for _, policy := range policies {
		lower := common.ToLowerUnicode(policy.Keyword)

		switch policy.Mode {
		case pb.BlacklistMatchMode_BLACKLIST_MATCH_MODE_WHOLE:
			b.wholeWords[lower] = struct{}{}
			fallthrough
		case pb.BlacklistMatchMode_BLACKLIST_MATCH_MODE_SUBSTRING:
			trieKeywords = append(trieKeywords, []rune(lower))
		case pb.BlacklistMatchMode_BLACKLIST_MATCH_MODE_REGEX:
			regex, err := regexp.Compile(lower)
			if err != nil {
				return fmt.Errorf(`encountered invalid regex %q when loading blacklist policies: %w`,
					policy.Keyword,
					err,
				)
			}

			b.regexes = append(b.regexes, regex)
		default:
			return fmt.Errorf(`encountered unknown match mode %d when loading blacklist policies`, policy.Mode)
		}
	}

	if len(trieKeywords) > 0 {
		err = b.machine.Build(trieKeywords)
		if err != nil {
			return err
		}
		b.hasTrieKeywords = true
	}

	return nil
}

// ErrEmptyKeyword is returned when trying to create a policy with an empty keyword.
var ErrEmptyKeyword = errors.New("tried to add blacklist policy with empty keyword")

// AddPolicies will add blacklist policies to the database and then update the string matching engine.
// It does not validate policies other than making sure that their keywords aren't empty.
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
// Haystack (both the []rune version and the string version) should be in lowercase.
func (b *Blacklist) Match(haystackRunes []rune, haystackStr string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Try words first.
	if b.hasTrieKeywords {
		results := b.machine.MultiPatternSearch(haystackRunes, true)

		for _, result := range results {
			wordStr := string(result.Word)

			// Got a match.
			// If this keyword should only match whole words, use that logic instead.
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
					startChar := haystackRunes[wordStart]
					prevChar := haystackRunes[wordStart-1]

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

				if wordEnd < len(haystackRunes)-1 {
					endChar := haystackRunes[wordEnd]
					nextChar := haystackRunes[wordEnd+1]

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

				return true
			}

			return true
		}
	}

	// No words matched, try regex.
	for _, regex := range b.regexes {
		if regex.MatchString(haystackStr) {
			return true
		}
	}

	return false
}
