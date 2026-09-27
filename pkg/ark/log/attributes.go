package log

import (
	"log/slog"
	"strconv"
	"time"
)

type Attribute struct {
	Key   string
	Value string
}

type Attributes []Attribute

func (as *Attributes) AppendAttr(attr slog.Attr, group string) {
	key := attr.Key
	if key == "" {
		return
	}

	if group != "" {
		key = group + "." + key
	}

	if attr.Value.Kind() == slog.KindGroup {
		attrs := attr.Value.Group()
		if len(attrs) == 0 {
			return
		}

		for _, attr := range attrs {
			as.AppendAttr(attr, key)
		}
	} else {
		if value := FormatAttributeValue(attr.Value); value != "" {
			*as = append(*as, Attribute{Key: key, Value: value})
		}
	}
}

func FormatAttributeValue(value slog.Value) (s string) {
	switch value.Kind() {
	case slog.KindAny:

	case slog.KindBool:
		s = strconv.FormatBool(value.Bool())

	case slog.KindDuration:
		s = strconv.Quote(value.Duration().String())

	case slog.KindFloat64:
		s = strconv.FormatFloat(value.Float64(), 'f', -1, 64)

	case slog.KindInt64:
		s = strconv.FormatInt(value.Int64(), 10)

	case slog.KindString:
		s = FormatStringValue(value.String())

	case slog.KindTime:
		s = value.Time().Format(time.RFC3339)

	case slog.KindUint64:
		s = strconv.FormatUint(value.Uint64(), 10)

	case slog.KindLogValuer:
		s = FormatAttributeValue(value.LogValuer().LogValue())

	default:
		s = value.String()
	}

	return
}

func FormatStringValue(s string) string {
	if s == "" {
		return "\"\""
	}

	for _, r := range s {
		if r == ' ' || r == '"' || r == '\\' || !strconv.IsPrint(r) {
			return strconv.Quote(s)
		}
	}

	return s
}
