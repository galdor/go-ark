package pg

import (
	"testing"

	"go.n16f.net/check"
)

func TestQuoteString(t *testing.T) {
	check := check.New(t)

	check.Equal(QuoteString(``), `''`)
	check.Equal(QuoteString(`foo`), `'foo'`)
	check.Equal(QuoteString(`foo bar`), `'foo bar'`)
	check.Equal(QuoteString(`foo 'bar'`), `'foo ''bar'''`)
	check.Equal(QuoteString(`''`), `''''''`)
}

func TestQuoteIdentifier(t *testing.T) {
	check := check.New(t)

	check.Equal(QuoteIdentifier(`foo`), `foo`)
	check.Equal(QuoteIdentifier(`123foo`), `"123foo"`)
	check.Equal(QuoteIdentifier(`foo "bar" baz`), `"foo ""bar"" baz"`)
	check.Equal(QuoteIdentifier(`foo bar baz`), `"foo bar baz"`)
	check.Equal(QuoteIdentifier(`"hello"`), `"""hello"""`)
	check.Equal(QuoteIdentifier(`""`), `""""""`)
}
