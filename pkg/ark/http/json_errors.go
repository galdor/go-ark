package http

import (
	"encoding/json/jsontext"
	"fmt"

	"go.n16f.net/ark/pkg/ark/json"
)

// We keep a raw copy of the error data object so that it can be decoded to a
// specific Go type if necessary.

type JSONError struct {
	Code    string         `json:"code,omitempty"`
	Message string         `json:"message"`
	RawData jsontext.Value `json:"data,omitempty"`
	Data    ErrorData      `json:"-"`
}

type ValidationJSONErrorData struct {
	ValidationErrors json.ValidationErrors `json:"validation_errors"`
}

func (err *JSONError) MarshalJSON() ([]byte, error) {
	type JSONError2 JSONError

	err2 := JSONError2(*err)

	if err2.Data != nil {
		var dataErr error

		err2.RawData, dataErr = json.Encode(err2.Data)
		if dataErr != nil {
			return nil, fmt.Errorf("cannot encode error data: %w", dataErr)
		}
	}

	return json.Encode(err2)
}

func (err *JSONError) UnmarshalJSON(data []byte) error {
	type JSONError2 JSONError

	err2 := JSONError2(*err)

	if err3 := json.Decode(data, &err2); err3 != nil {
		return err3
	}

	if err2.RawData != nil {
		dataErr := json.Decode(err2.RawData, &err2.Data)
		if dataErr != nil {
			return fmt.Errorf("cannot decode error data: %w", dataErr)
		}
	}

	*err = JSONError(err2)

	return nil
}

func (err *JSONError) Error() string {
	return err.Message
}

func (err *JSONError) DecodeData(target any) error {
	if err.RawData == nil {
		return fmt.Errorf("missing or empty error data")
	}

	return json.Decode(err.RawData, target)
}
