// Package zstd is a stub satisfying kafka-go's dependency on
// github.com/klauspost/compress/zstd. Zstd compression is not supported
// in this build; messages using zstd will fail to decode/encode.
package zstd

import (
	"errors"
	"io"
)

var errNotSupported = errors.New("zstd compression not supported")

// EncoderLevel mirrors the type used in option functions.
type EncoderLevel int

// EncoderLevelFromZstd converts a numeric zstd level to an EncoderLevel.
func EncoderLevelFromZstd(level int) EncoderLevel { return EncoderLevel(level) }

// Option types used in constructor functions.
type DecoderOption func()
type EncoderOption func()

func WithDecoderConcurrency(n int) DecoderOption   { return func() {} }
func WithEncoderLevel(l EncoderLevel) EncoderOption { return func() {} }
func WithEncoderConcurrency(n int) EncoderOption    { return func() {} }
func WithZeroFrames(b bool) EncoderOption           { return func() {} }

// Decoder is a stub zstd decoder.
type Decoder struct{}

func NewReader(r io.Reader, opts ...DecoderOption) (*Decoder, error) {
	return &Decoder{}, nil
}

func (d *Decoder) Reset(r io.Reader) {
	// no-op
}

func (d *Decoder) Read(p []byte) (int, error) {
	return 0, errNotSupported
}

func (d *Decoder) WriteTo(w io.Writer) (int64, error) {
	return 0, errNotSupported
}

func (d *Decoder) Close() {}

// Encoder is a stub zstd encoder.
type Encoder struct{}

func NewWriter(w io.Writer, opts ...EncoderOption) (*Encoder, error) {
	return &Encoder{}, nil
}

func (e *Encoder) Close() error { return nil }

func (e *Encoder) Reset(w io.Writer) {}

func (e *Encoder) Write(p []byte) (int, error) {
	return 0, errNotSupported
}

func (e *Encoder) ReadFrom(r io.Reader) (int64, error) {
	return 0, errNotSupported
}
