package utils

import (
	"testing"

	"go.n16f.net/check"
)

func TestEscapeNonPrintable(t *testing.T) {
	check := check.New(t)

	tests := []struct {
		s  string
		es string
	}{
		{"", ``},
		{"foo", `foo`},
		{"foo\\bar", `foo\\bar`},
		{"foo\nbar", `foo\nbar`},
		{"\a-\b-\x7f", `\a-\b-\x7f`},
	}

	for _, test := range tests {
		es := EscapeNonPrintable(test.s)
		check.Equal(es, test.es, test.s)
	}
}
