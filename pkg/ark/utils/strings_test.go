package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeNonPrintable(t *testing.T) {
	assert := assert.New(t)

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
		assert.Equal(test.es, es, test.s)
	}
}
