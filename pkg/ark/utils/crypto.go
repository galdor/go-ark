package utils

import (
	"crypto/rand"
	"crypto/subtle"
)

func RandomBytes(n int) []byte {
	data := make([]byte, n)

	if _, err := rand.Read(data); err != nil {
		Panic("cannot generate random data: %v", err)
	}

	return data
}

func EqualBytesConstantTime(data1, data2 []byte) bool {
	return subtle.ConstantTimeCompare(data1, data2) == 1
}

func EqualStringsConstantTime(s1, s2 string) bool {
	return EqualBytesConstantTime([]byte(s1), []byte(s2))
}
