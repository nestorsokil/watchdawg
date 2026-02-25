// Package lz4 is a stub satisfying kafka-go's dependency on
// github.com/pierrec/lz4/v4. LZ4 compression is not supported in this build;
// messages using lz4 will fail to decode/encode.
package lz4

import (
	"errors"
	"io"
)

var errNotSupported = errors.New("lz4 compression not supported")

// Reader is a stub lz4 reader.
type Reader struct{}

func NewReader(r io.Reader) *Reader { return &Reader{} }

func (r *Reader) Reset(rd io.Reader) {}

func (r *Reader) Read(p []byte) (int, error) {
	return 0, errNotSupported
}

// Writer is a stub lz4 writer.
type Writer struct{}

func NewWriter(w io.Writer) *Writer { return &Writer{} }

func (w *Writer) Reset(wr io.Writer) {}

func (w *Writer) Write(p []byte) (int, error) {
	return 0, errNotSupported
}

func (w *Writer) Close() error { return nil }
