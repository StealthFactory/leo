package store

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"strings"
	"unicode"
)

// Match tiers, best first.
const (
	tierExact     = iota // the key is the term
	tierPrefix           // the key starts with the term
	tierWord             // a word inside the key starts with the term
	tierSubstring        // the key contains the term
	tierFuzzy            // the key contains the term's characters in order
	tierValue            // only the (non-secret) value contains the term
)

// wordSeparators mark the start of a new word inside a key (deploy.host,
// api-key, aws_region, team/name, ...).
const wordSeparators = ".-_/: @#"

// Search returns the keys matching term, ignoring case, best matches first:
// exact key, key prefix, word start inside the key, key substring, key
// subsequence (dh finds deploy.host), and finally keys whose value contains
// the term. Secret values are never searched. Ties go to the earlier (or, for
// subsequences, tighter) match, then alphabetical order.
func (s *Store) Search(term string) []string {
	needle := lowerRunes(term)
	type hit struct {
		key         string
		tier, score int
	}
	var hits []hit
	for k, v := range s.data {
		tier, score, ok := matchKey([]rune(k), needle)
		if !ok {
			if s.secrets[k] || !strings.Contains(string(lowerRunes(valueText(v))), string(needle)) {
				continue
			}
			tier, score = tierValue, 0
		}
		hits = append(hits, hit{k, tier, score})
	}
	slices.SortFunc(hits, func(a, b hit) int {
		return cmp.Or(cmp.Compare(a.tier, b.tier), cmp.Compare(a.score, b.score), strings.Compare(a.key, b.key))
	})
	keys := make([]string, len(hits))
	for i, h := range hits {
		keys[i] = h.key
	}
	return keys
}

// matchKey scores how key matches the lowercased needle. score orders matches
// within a tier: the match position, or the span of a subsequence match.
func matchKey(key, needle []rune) (tier, score int, ok bool) {
	lower := lowerRunes(string(key))
	if slices.Equal(lower, needle) {
		return tierExact, 0, true
	}
	if hasPrefixAt(lower, 0, needle) {
		return tierPrefix, 0, true
	}
	substring := -1
	for i := 1; i+len(needle) <= len(lower); i++ {
		if !hasPrefixAt(lower, i, needle) {
			continue
		}
		if isWordStart(key, i) {
			return tierWord, i, true
		}
		if substring < 0 {
			substring = i
		}
	}
	if substring >= 0 {
		return tierSubstring, substring, true
	}
	if span := subsequenceSpan(lower, needle); span > 0 {
		return tierFuzzy, span, true
	}
	return 0, 0, false
}

func hasPrefixAt(s []rune, i int, prefix []rune) bool {
	return i+len(prefix) <= len(s) && slices.Equal(s[i:i+len(prefix)], prefix)
}

// isWordStart reports whether key[i] begins a word: it follows a separator or
// is a camelCase hump (deployHost).
func isWordStart(key []rune, i int) bool {
	prev := key[i-1]
	return strings.ContainsRune(wordSeparators, prev) ||
		(unicode.IsLower(prev) && unicode.IsUpper(key[i]))
}

// subsequenceSpan returns the length of the shortest stretch of s containing
// needle's characters in order, or 0 if there is none.
func subsequenceSpan(s, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	best := 0
	for start := range s {
		if s[start] != needle[0] {
			continue
		}
		j := 1
		end := start
		for i := start + 1; i < len(s) && j < len(needle); i++ {
			if s[i] == needle[j] {
				j++
				end = i
			}
		}
		if j < len(needle) {
			break // no later start can match either
		}
		if span := end - start + 1; best == 0 || span < best {
			best = span
		}
	}
	return best
}

// valueText is the searchable text of a value: a string's contents, or any
// other value as compact JSON.
func valueText(v json.RawMessage) string {
	if Kind(v) == "string" {
		return UnquoteString(v)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, v); err != nil {
		return string(v)
	}
	return buf.String()
}

func lowerRunes(s string) []rune {
	r := []rune(s)
	for i, c := range r {
		r[i] = unicode.ToLower(c)
	}
	return r
}
