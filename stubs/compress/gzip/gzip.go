// Package gzip wraps the standard library compress/gzip to satisfy kafka-go's
// dependency on github.com/klauspost/compress/gzip (which shares the same API).
package gzip

import (
	"compress/gzip"
	"io"
)

type Reader = gzip.Reader
type Writer = gzip.Writer

const DefaultCompression = gzip.DefaultCompression

func NewReader(r io.Reader) (*gzip.Reader, error) {
	return gzip.NewReader(r)
}

func NewWriterLevel(w io.Writer, level int) (*gzip.Writer, error) {
	return gzip.NewWriterLevel(w, level)
}
