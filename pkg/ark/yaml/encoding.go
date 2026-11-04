package yaml

import (
	"bytes"
	"io"

	goyaml "github.com/goccy/go-yaml"
)

type EncodingOptions struct {
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
	opts2 := []goyaml.EncodeOption{
		goyaml.Indent(2),
		goyaml.IndentSequence(false),
		goyaml.UseSingleQuote(false),
	}

	data, err := goyaml.MarshalWithOptions(value, opts2...)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, bytes.NewReader(data))
	return err
}
