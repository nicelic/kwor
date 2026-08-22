package compressionalgorithm

import (
	"io"

	"github.com/klauspost/compress/s2"
)

func newS2Decoder(source io.Reader) (io.Reader, io.Closer, error) {
	return s2.NewReader(source), closeOnly{}, nil
}

func newS2Encoder(dst io.Writer, level int) (io.WriteCloser, error) {
	if level >= 7 {
		return s2.NewWriter(dst, s2.WriterBestCompression()), nil
	}
	if level >= 4 {
		return s2.NewWriter(dst, s2.WriterBetterCompression()), nil
	}
	return s2.NewWriter(dst), nil
}
