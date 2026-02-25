// Package s2 is a stub satisfying kafka-go's dependency on
// github.com/klauspost/compress/s2. S2/snappy compression is not supported
// in this build.
package s2

// EncodeSnappy, EncodeSnappyBetter, EncodeSnappyBest are the encoder function
// signatures expected by kafka-go/compress/snappy.
func EncodeSnappy(dst, src []byte) []byte       { return src }
func EncodeSnappyBetter(dst, src []byte) []byte { return src }
func EncodeSnappyBest(dst, src []byte) []byte   { return src }
