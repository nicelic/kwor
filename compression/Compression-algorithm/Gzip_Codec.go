package compressionalgorithm

import (
	"io"

	"github.com/klauspost/compress/gzip"
)

func newGzipDecoder(source io.Reader) (io.Reader, io.Closer, error) {
	decoder, err := gzip.NewReader(source)
	if err != nil {
		return nil, nil, err
	}
	return decoder, decoder, nil
}

func newGzipEncoder(dst io.Writer, level int) (io.WriteCloser, error) {
	return gzip.NewWriterLevel(dst, normalizeDeflateLevel(level))
}
