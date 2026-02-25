// Package snappy is a stub satisfying kafka-go's dependency on
// github.com/klauspost/compress/snappy. Snappy compression is not supported
// in this build; messages using snappy will fail to decode.
package snappy

import "errors"

var errNotSupported = errors.New("snappy compression not supported")

// Decode is the decode function signature expected by kafka-go/compress/snappy.
func Decode(dst, src []byte) ([]byte, error) {
	return nil, errNotSupported
}

// Encode is the encode function signature expected by kafka-go/compress/snappy.
func Encode(dst, src []byte) []byte {
	return src // pass-through; Kafka producer won't use this codec
}

// DecodedLen is used by kafka-go/compress/snappy/xerial.go to size the output buffer.
func DecodedLen(src []byte) (int, error) {
	return 0, errNotSupported
}
