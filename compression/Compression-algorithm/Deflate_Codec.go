package compressionalgorithm

import (
	"io"

	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/zlib"
)

func newDeflateDecoder(source io.Reader) (io.Reader, io.Closer, error) {
	decoder, err := zlib.NewReader(source)
	if err != nil {
		return nil, nil, err
	}
	return decoder, decoder, nil
}

func newDeflateEncoder(dst io.Writer, level int) (io.WriteCloser, error) {
	return zlib.NewWriterLevel(dst, normalizeDeflateLevel(level))
}

func normalizeDeflateLevel(level int) int {
	if level < flate.HuffmanOnly {
		return flate.HuffmanOnly
	}
	if level > flate.BestCompression {
		return flate.BestCompression
	}
	return level
}
