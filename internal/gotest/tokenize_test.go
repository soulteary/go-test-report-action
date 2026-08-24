package gotest

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"spaces-only", "   ", nil},
		{"simple", "-run TestFoo", []string{"-run", "TestFoo"}},
		{"multi-space", "-run   TestFoo\t-v", []string{"-run", "TestFoo", "-v"}},
		{"single-quotes", "-run 'Test Foo Bar'", []string{"-run", "Test Foo Bar"}},
		{"double-quotes", `-run "Test Foo"`, []string{"-run", "Test Foo"}},
		{"double-quote-escape", `-run "a\"b"`, []string{"-run", `a"b`}},
		{"backslash-escape", `a\ b`, []string{"a b"}},
		{"mixed", `-run 'A B' -x "c d" e`, []string{"-run", "A B", "-x", "c d", "e"}},
		{"adjacent-quotes", `'a'"b"c`, []string{"abc"}},
		{"newline-separates", "a\nb", []string{"a", "b"}},
		{"double-quote-literal-backslash", `"a\b"`, []string{`a\b`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Tokenize(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Tokenize(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestTokenize_UnterminatedQuote(t *testing.T) {
	for _, in := range []string{`'abc`, `"abc`, `-run "a b`, `"a\`} {
		if _, err := Tokenize(in); err == nil {
			t.Fatalf("expected error for unterminated quote %q", in)
		}
	}
}
