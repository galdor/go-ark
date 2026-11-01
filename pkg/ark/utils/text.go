package utils

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var terminalPunctuation = regexp.MustCompile(`[.!?…]["'”’»)\]]*$`)

func ToSentence(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return trimmed
	}

	c, size := utf8.DecodeRuneInString(trimmed)
	first := trimmed[:size]
	if c != utf8.RuneError {
		first = string(unicode.ToUpper(c))
	}

	suffix := "."
	if terminalPunctuation.MatchString(trimmed) {
		suffix = ""
	}

	return first + trimmed[size:] + suffix
}
