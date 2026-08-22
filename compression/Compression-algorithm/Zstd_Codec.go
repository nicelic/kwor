package compressionalgorithm

import (
	"io"

	"github.com/klauspost/compress/zstd"
)

const httpZstdMaximumWindowBytes int64 = 8 << 20

func newZstdDecoder(source io.Reader, maxDecodedBytes int64) (io.Reader, io.Closer, error) {
	maxWindow := httpZstdMaximumWindowBytes
	if maxDecodedBytes > 0 && maxDecodedBytes < maxWindow {
		maxWindow = maxDecodedBytes
	}
	if maxWindow < zstd.MinWindowSize {
		maxWindow = zstd.MinWindowSize
	}
	decoder, err := zstd.NewReader(source,
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderMaxWindow(uint64(maxWindow)),
	)
	if err != nil {
		return nil, nil, err
	}
	return decoder, closeOnly{closeFn: func() error {
		decoder.Close()
		return nil
	}}, nil
}

func newZstdEncoder(dst io.Writer, level int) (io.WriteCloser, error) {
	// klauspost/compress exposes stable presets rather than the reference
	// implementation's numeric 1-22 levels. Level 5 maps to its default,
	// high-throughput preset.
	return zstd.NewWriter(dst,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
		// RFC 9659 requires HTTP zstd content-coding frames to use a window
		// no larger than 8 MiB for interoperability with browser decoders.
		zstd.WithWindowSize(int(httpZstdMaximumWindowBytes)),
		zstd.WithEncoderConcurrency(1),
	)
}
