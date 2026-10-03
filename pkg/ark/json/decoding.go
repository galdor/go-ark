package json

import (
	"encoding/json/v2"
	json2 "encoding/json/v2"
)

type DecodingOptions struct {
	Strict bool
}

func Decode(data []byte, dest any) error {
	return DecodeWithOptions(data, dest, DecodingOptions{})
}

func DecodeWithOptions(data []byte, dest any, opts DecodingOptions) error {
	var opts2 []json2.Options

	if opts.Strict {
		opts2 = append(opts2, json2.RejectUnknownMembers(true))
	}

	if err := json.Unmarshal(data, dest, opts2...); err != nil {
		return err
	}

	return Validate(dest)
}
