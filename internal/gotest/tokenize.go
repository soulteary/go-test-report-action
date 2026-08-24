package gotest

import (
	"fmt"
	"strings"
)

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
	var tokens []string
	var cur strings.Builder
	inToken := false

	const (
		none = iota
		single
		double
	)
	quote := none

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch quote {
		case single:
			if c == '\'' {
				quote = none
			} else {
				cur.WriteRune(c)
			}
		case double:
			switch {
			case c == '\\' && i+1 < len(runes):
				next := runes[i+1]
				if next == '"' || next == '\\' {
					cur.WriteRune(next)
					i++
					continue
				}
				cur.WriteRune(c)
			case c == '"':
				quote = none
			default:
				cur.WriteRune(c)
			}
		default: // none
			switch {
			case c == '\'':
				quote = single
				inToken = true
			case c == '"':
				quote = double
				inToken = true
			case c == '\\' && i+1 < len(runes):
				cur.WriteRune(runes[i+1])
				i++
				inToken = true
			case c == ' ' || c == '\t' || c == '\n' || c == '\r':
				if inToken {
					tokens = append(tokens, cur.String())
					cur.Reset()
					inToken = false
				}
			default:
				cur.WriteRune(c)
				inToken = true
			}
		}
	}

	if quote != none {
		return nil, fmt.Errorf("unterminated quote in test_args: %q", s)
	}
	if inToken {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}
