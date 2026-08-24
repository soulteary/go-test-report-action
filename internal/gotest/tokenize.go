package gotest

import (
	"fmt"
	"strings"
)

// quote state constants for the tokenizer.
const (
	quoteNone = iota
	quoteSingle
	quoteDouble
)

// tokenizer holds the mutable state of a single Tokenize pass.
type tokenizer struct {
	runes   []rune
	tokens  []string
	cur     strings.Builder
	inToken bool
	quote   int
}

// Tokenize splits a shell-like argument string into individual arguments
// without invoking a shell. It supports:
//   - whitespace separation (spaces, tabs, newlines)
//   - single quotes: everything literal until the next single quote
//   - double quotes: literal except backslash escapes of \" and \\
//   - backslash escaping outside quotes
//
// It intentionally does NOT perform variable expansion, globbing, command
// substitution, or any other shell feature. This keeps `test_args` safe to
// pass directly as exec arguments.
func Tokenize(s string) ([]string, error) {
	t := &tokenizer{runes: []rune(s), quote: quoteNone}

	for i := 0; i < len(t.runes); i++ {
		switch t.quote {
		case quoteSingle:
			t.stepSingle(t.runes[i])
		case quoteDouble:
			i = t.stepDouble(i)
		default:
			i = t.stepNone(i)
		}
	}

	if t.quote != quoteNone {
		return nil, fmt.Errorf("unterminated quote in test_args: %q", s)
	}
	if t.inToken {
		t.tokens = append(t.tokens, t.cur.String())
	}
	return t.tokens, nil
}

func isSpace(c rune) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// stepSingle handles a rune inside single quotes (everything literal).
func (t *tokenizer) stepSingle(c rune) {
	if c == '\'' {
		t.quote = quoteNone
		return
	}
	t.cur.WriteRune(c)
}

// stepDouble handles a rune inside double quotes and returns the new index.
func (t *tokenizer) stepDouble(i int) int {
	c := t.runes[i]
	switch {
	case c == '\\' && i+1 < len(t.runes):
		next := t.runes[i+1]
		if next == '"' || next == '\\' {
			t.cur.WriteRune(next)
			return i + 1
		}
		t.cur.WriteRune(c)
	case c == '"':
		t.quote = quoteNone
	default:
		t.cur.WriteRune(c)
	}
	return i
}

// stepNone handles a rune outside any quotes and returns the new index.
func (t *tokenizer) stepNone(i int) int {
	c := t.runes[i]
	switch {
	case c == '\'':
		t.quote = quoteSingle
		t.inToken = true
	case c == '"':
		t.quote = quoteDouble
		t.inToken = true
	case c == '\\' && i+1 < len(t.runes):
		t.cur.WriteRune(t.runes[i+1])
		t.inToken = true
		return i + 1
	case isSpace(c):
		if t.inToken {
			t.tokens = append(t.tokens, t.cur.String())
			t.cur.Reset()
			t.inToken = false
		}
	default:
		t.cur.WriteRune(c)
		t.inToken = true
	}
	return i
}
