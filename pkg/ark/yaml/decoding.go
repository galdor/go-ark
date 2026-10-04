package yaml

import (
	goyaml "github.com/goccy/go-yaml"
	"go.n16f.net/ark/pkg/ark/json"
)

type DecodingOptions struct {
	Strict bool
}

func Decode(data []byte, dest any) error {
	return DecodeWithOptions(data, dest, DecodingOptions{})
}

func DecodeWithOptions(data []byte, dest any, opts DecodingOptions) error {
	var opts2 []goyaml.DecodeOption

	if opts.Strict {
		opts2 = append(opts2, goyaml.DisallowUnknownField())
	}

	if err := goyaml.UnmarshalWithOptions(data, dest, opts2...); err != nil {
		return err
	}

	return json.Validate(dest)
}
