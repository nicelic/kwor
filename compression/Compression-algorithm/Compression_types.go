package compressionalgorithm

import "io"

// Algorithm is an HTTP content-coding supported by the reverse proxy.
type Algorithm string

const (
	AlgorithmIdentity Algorithm = "identity"
	AlgorithmZstd     Algorithm = "zstd"
	AlgorithmS2       Algorithm = "s2"
	AlgorithmSnappy   Algorithm = "snappy"
	AlgorithmBrotli   Algorithm = "br"
	AlgorithmDeflate  Algorithm = "deflate"
	AlgorithmGzip     Algorithm = "gzip"
)

const (
	// DefaultLevel is the project's zstd compression level. Other codecs use
	// DefaultGeneralLevel because their level models and interoperability
	// limits differ from zstd's.
	DefaultLevel = 8

	// DefaultGeneralLevel is the default for Brotli, S2, DEFLATE and Gzip.
	// Snappy ignores numeric levels by design.
	DefaultGeneralLevel = 6

	// DefaultMinimumResponseSize avoids adding a compression header to very
	// small responses where the framing overhead is larger than the saving.
	DefaultMinimumResponseSize int64 = 256
)

// DefaultLevelFor returns the default numeric input for one codec. A zero
// level passed to NewEncoder or HTTPResponseWriter selects this value.
func DefaultLevelFor(algorithm Algorithm) int {
	if algorithm == AlgorithmZstd {
		return DefaultLevel
	}
	return DefaultGeneralLevel
}

// fixedPriorityOrder returns a fresh copy of the only order used by the
// negotiation implementation. Keeping the source order here prevents a
// mutable exported compatibility value from changing runtime selection.
func fixedPriorityOrder() [6]Algorithm {
	return [...]Algorithm{
		AlgorithmZstd,
		AlgorithmS2,
		AlgorithmSnappy,
		AlgorithmBrotli,
		AlgorithmDeflate,
		AlgorithmGzip,
	}
}

// Priority is retained as a public compatibility snapshot for callers and
// tests that enumerate the supported algorithms. Runtime negotiation and
// upstream advertisement use fixedPriorityOrder, so mutating this snapshot
// cannot reorder the project's wire behavior. Positive qvalues are treated as
// support flags; q=0 is refusal, and the magnitude of a positive qvalue does
// not reorder the fixed list.
var Priority = fixedPriorityOrder()

// ErrUnsupportedEncoding means that a Content-Encoding token is not handled
// by this project.
var ErrUnsupportedEncoding = unsupportedEncodingError{}

// ErrDecodedSizeLimit means that a compressed request or response expanded
// beyond the configured safety limit.
var ErrDecodedSizeLimit = decodedSizeLimitError{}

type unsupportedEncodingError struct{}

func (unsupportedEncodingError) Error() string { return "unsupported content encoding" }

type decodedSizeLimitError struct{}

func (decodedSizeLimitError) Error() string { return "decoded content exceeds the configured limit" }

type identityWriteCloser struct {
	io.Writer
}

func (identityWriteCloser) Close() error { return nil }

// EncoderFlusher is implemented by the streaming encoders that can emit a
// complete prefix without ending the compressed stream.
type EncoderFlusher interface {
	Flush() error
}
