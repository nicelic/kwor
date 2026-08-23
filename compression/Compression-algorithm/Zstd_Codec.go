package compressionalgorithm

import (
	"io"

	"github.com/klauspost/compress/zstd"
)

// zstd encoder windows must be powers of two. The requested 36 MiB policy is
// therefore rounded down to the largest legal value that does not exceed it.
const httpZstdMaximumWindowBytes int64 = 32 << 20

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

func newZstdEncoder(dst io.Writer, level int, windowBytes int64) (io.WriteCloser, error) {
	// klauspost/compress exposes stable presets rather than the reference
	// implementation's numeric 1-22 levels. The project's level 8 maps to the
	// library's better-compression preset (roughly reference levels 7-8).
	options := []zstd.EOption{
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
		zstd.WithEncoderConcurrency(1),
	}
	if windowBytes > 0 {
		options = append(options, zstd.WithWindowSize(int(windowBytes)))
	}
	// Unknown-length streams keep the codec's own default window. Known large
	// responses are bounded by encoderWindowBytesForContentLength.
	return zstd.NewWriter(dst, options...)
}
