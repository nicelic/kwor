package compressionalgorithm

import (
	"io"

	"github.com/andybalholm/brotli"
)

func newBrotliDecoder(source io.Reader) (io.Reader, io.Closer, error) {
	return brotli.NewReader(source), closeOnly{}, nil
}

func newBrotliEncoder(dst io.Writer, level int) (io.WriteCloser, error) {
	if level > brotli.BestCompression {
		level = brotli.BestCompression
	}
	return brotli.NewWriterLevel(dst, level), nil
}
