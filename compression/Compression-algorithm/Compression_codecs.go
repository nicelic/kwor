package compressionalgorithm

import "io"

type decoderReadCloser struct {
	io.Reader
	source   io.Closer
	decoders []io.Closer
	limit    int64
	decoded  int64
}

func (r *decoderReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.limit > 0 {
		remaining := r.limit - r.decoded
		if remaining <= 0 {
			// Probe one decoded byte so a payload whose size is exactly the
			// configured limit can still finish with EOF. If a byte exists,
			// report the limit without exposing it to the caller.
			var probe [1]byte
			n, err := r.Reader.Read(probe[:])
			if n > 0 {
				return 0, ErrDecodedSizeLimit
			}
			return 0, err
		}
		if int64(len(p)) > remaining {
			p = p[:int(remaining)]
		}
	}
	n, err := r.Reader.Read(p)
	r.decoded += int64(n)
	return n, err
}

func (r *decoderReadCloser) Close() error {
	var firstErr error
	for index := len(r.decoders) - 1; index >= 0; index-- {
		if err := r.decoders[index].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.source != nil {
		if err := r.source.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type closeOnly struct {
	closeFn func() error
}

func (c closeOnly) Close() error {
	if c.closeFn == nil {
		return nil
	}
	return c.closeFn()
}

// NewDecoder creates a streaming decoder for a Content-Encoding header.
// maxDecodedBytes is zero for unlimited streaming output.
func NewDecoder(source io.ReadCloser, header string, maxDecodedBytes int64) (io.ReadCloser, error) {
	if source == nil {
		return nil, io.ErrClosedPipe
	}
	algorithms, err := ParseContentEncoding(header)
	if err != nil {
		return nil, err
	}
	if len(algorithms) == 0 {
		return &decoderReadCloser{Reader: source, source: source, limit: maxDecodedBytes}, nil
	}

	var current io.Reader = source
	decoders := make([]io.Closer, 0, len(algorithms))
	for index := len(algorithms) - 1; index >= 0; index-- {
		decoder, closer, createErr := newDecoderForAlgorithm(current, algorithms[index], maxDecodedBytes)
		if createErr != nil {
			for closeIndex := len(decoders) - 1; closeIndex >= 0; closeIndex-- {
				_ = decoders[closeIndex].Close()
			}
			_ = source.Close()
			return nil, createErr
		}
		current = decoder
		decoders = append(decoders, closer)
	}
	return &decoderReadCloser{
		Reader:   current,
		source:   source,
		decoders: decoders,
		limit:    maxDecodedBytes,
	}, nil
}

func newDecoderForAlgorithm(source io.Reader, algorithm Algorithm, maxDecodedBytes int64) (io.Reader, io.Closer, error) {
	switch algorithm {
	case AlgorithmZstd:
		return newZstdDecoder(source, maxDecodedBytes)
	case AlgorithmS2:
		return newS2Decoder(source)
	case AlgorithmSnappy:
		return newSnappyDecoder(source)
	case AlgorithmBrotli:
		return newBrotliDecoder(source)
	case AlgorithmDeflate:
		return newDeflateDecoder(source)
	case AlgorithmGzip:
		return newGzipDecoder(source)
	default:
		return nil, nil, ErrUnsupportedEncoding
	}
}

// NewEncoder creates a streaming encoder using the project's default level
// mapping. The returned writer does not close dst.
func NewEncoder(dst io.Writer, algorithm Algorithm, level int) (io.WriteCloser, error) {
	if dst == nil {
		return nil, io.ErrClosedPipe
	}
	if algorithm == AlgorithmIdentity {
		return identityWriteCloser{Writer: dst}, nil
	}
	if level <= 0 {
		level = DefaultLevel
	}
	switch algorithm {
	case AlgorithmZstd:
		return newZstdEncoder(dst, level)
	case AlgorithmS2:
		return newS2Encoder(dst, level)
	case AlgorithmSnappy:
		return newSnappyEncoder(dst, level)
	case AlgorithmBrotli:
		return newBrotliEncoder(dst, level)
	case AlgorithmDeflate:
		return newDeflateEncoder(dst, level)
	case AlgorithmGzip:
		return newGzipEncoder(dst, level)
	default:
		return nil, ErrUnsupportedEncoding
	}
}
