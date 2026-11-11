package base64

import "encoding/base64"

// This package only exists due to how verbose and frustrating the standard API
// is.

func Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func EncodeString(data string) string {
	return Encode([]byte(data))
}

func URLEncode(data []byte) string {
	return base64.URLEncoding.EncodeToString(data)
}

func URLEncodeString(data string) string {
	return URLEncode([]byte(data))
}

func Decode(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

func DecodeString(data string) (string, error) {
	buf := make([]byte, base64.StdEncoding.DecodedLen(len(data)))
	n, err := base64.StdEncoding.Decode(buf, []byte(data))
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

func URLDecode(data string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(data)
}

func URLDecodeString(data string) (string, error) {
	buf := make([]byte, base64.URLEncoding.DecodedLen(len(data)))
	n, err := base64.URLEncoding.Decode(buf, []byte(data))
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}
