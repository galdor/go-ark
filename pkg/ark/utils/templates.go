package utils

import (
	"cmp"
	"os"
	"strconv"
	"strings"
)

var TemplateFunctions = map[string]interface{}{
	"env": os.Getenv,

	"env2": func(name, defaultValue string) string {
		return cmp.Or(os.Getenv(name), defaultValue)
	},

	"quote": func(s string) string {
		return strconv.Quote(s)
	},

	"split": func(sep, s string) []string {
		return strings.Split(s, sep)
	},
}
