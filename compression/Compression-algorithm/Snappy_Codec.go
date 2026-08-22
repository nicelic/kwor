package compressionalgorithm

import (
	"io"

	"github.com/golang/snappy"
)

func newSnappyDecoder(source io.Reader) (io.Reader, io.Closer, error) {
	return snappy.NewReader(source), closeOnly{}, nil
}

func newSnappyEncoder(dst io.Writer, _ int) (io.WriteCloser, error) {
	return snappy.NewBufferedWriter(dst), nil
}
