package json

import (
	"bytes"
	"encoding/json/jsontext"
	json2 "encoding/json/v2"
	"io"
)

type EncodingOptions struct {
	Indent bool
}

func Encode(value any) ([]byte, error) {
	return EncodeWithOptions(value, EncodingOptions{})
}

func EncodeWithOptions(value any, opts EncodingOptions) ([]byte, error) {
	var buf bytes.Buffer

	if err := EncodeToWithOptions(value, &buf, opts); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func EncodeTo(value any, w io.Writer) error {
	return EncodeToWithOptions(value, w, EncodingOptions{})
}

func EncodeToWithOptions(value any, w io.Writer, opts EncodingOptions) error {
	var opts2 []json2.Options

	if opts.Indent {
		opts2 = append(opts2, jsontext.WithIndent("  "))
	}

	return json2.MarshalWrite(w, value, opts2...)
}
