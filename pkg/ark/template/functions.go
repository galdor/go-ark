package template

import (
	"cmp"
	"os"
	"strconv"
	"strings"
)

var Functions = map[string]interface{}{
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

	"join": func(ss []string, sep string) string {
		return strings.Join(ss, sep)
	},
}
