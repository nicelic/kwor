package compressionalgorithm

import (
	"io"

	"github.com/andybalholm/brotli"
)

const httpBrotliMaximumWindowBits = 24

func newBrotliDecoder(source io.Reader) (io.Reader, io.Closer, error) {
	return brotli.NewReader(source), closeOnly{}, nil
}

func newBrotliEncoder(dst io.Writer, level int, windowBytes int64) (io.WriteCloser, error) {
	if level > brotli.BestCompression {
		level = brotli.BestCompression
	}
	options := brotli.WriterOptions{Quality: level}
	if windowBytes > 0 {
		bits := 10
		for (int64(1)<<bits) < windowBytes && bits < httpBrotliMaximumWindowBits {
			bits++
		}
		options.LGWin = bits
	}
	return brotli.NewWriterOptions(dst, options), nil
}
