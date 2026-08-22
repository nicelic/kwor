package compressionalgorithm

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// HTTPResponseOptions controls the streaming response wrapper.
type HTTPResponseOptions struct {
	Request *http.Request
	Level   int
	Enabled bool
	MinSize int64
	// AllowedAlgorithms limits local response negotiation. A nil value preserves
	// the historical all-algorithm behavior; a non-nil empty value allows only
	// identity. Callers can disable compression entirely with Enabled=false.
	AllowedAlgorithms []Algorithm
}

// HTTPResponseWriter compresses eligible response bodies after the upstream
// response headers have been received.  It preserves hijacking, flushing and
// HTTP/2 push capabilities required by ReverseProxy.
type HTTPResponseWriter struct {
	http.ResponseWriter
	request           *http.Request
	level             int
	minSize           int64
	enabled           bool
	allowedAlgorithms []Algorithm
	headerSent        bool
	statusSet         bool
	status            int
	algorithm         Algorithm
	encoder           io.WriteCloser
	encoderError      error
	closed            bool
	negotiationFailed bool
	bodyStarted       bool
	pending           []byte
	forceCompression  bool
}

func NewHTTPResponseWriter(writer http.ResponseWriter, options HTTPResponseOptions) *HTTPResponseWriter {
	minSize := options.MinSize
	if minSize < 0 {
		minSize = 0
	}
	if minSize == 0 && options.MinSize == 0 {
		// An explicit zero is useful for DoH, so keep it unchanged.
	}
	var allowedAlgorithms []Algorithm
	if options.AllowedAlgorithms != nil {
		allowedAlgorithms = append([]Algorithm{}, options.AllowedAlgorithms...)
	}
	return &HTTPResponseWriter{
		ResponseWriter:    writer,
		request:           options.Request,
		level:             options.Level,
		minSize:           minSize,
		enabled:           options.Enabled,
		allowedAlgorithms: allowedAlgorithms,
	}
}

func (w *HTTPResponseWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.statusSet || w.headerSent {
		return
	}
	w.statusSet = true
	w.status = status
}

func (w *HTTPResponseWriter) Write(p []byte) (int, error) {
	originalLength := len(p)
	if w.closed {
		return 0, io.ErrClosedPipe
	}
	if originalLength == 0 {
		return 0, nil
	}
	if !w.statusSet {
		w.status = http.StatusOK
		w.statusSet = true
	}
	w.bodyStarted = true
	if w.encoderError != nil {
		return 0, w.encoderError
	}
	if !w.headerSent && w.shouldBufferForMinimumSize() {
		remaining := int(w.minSize) - len(w.pending)
		if remaining > 0 {
			take := remaining
			if take > len(p) {
				take = len(p)
			}
			w.pending = append(w.pending, p[:take]...)
			p = p[take:]
			if len(p) == 0 {
				return originalLength, nil
			}
		}
	}
	if err := w.sendHeader(); err != nil {
		return 0, err
	}
	if w.negotiationFailed {
		// The 406 response has already been written by sendHeader. Report the
		// upstream bytes as consumed so ReverseProxy does not retry or emit a
		// second response on the same connection.
		return originalLength, nil
	}
	consumed := originalLength - len(p)
	if len(w.pending) > 0 {
		if _, err := w.writeBody(w.pending); err != nil {
			return consumed, err
		}
		w.pending = nil
	}
	written, err := w.writeBody(p)
	return consumed + written, err
}

func (w *HTTPResponseWriter) Close() error {
	if w == nil || w.closed {
		return nil
	}
	w.closed = true
	if !w.headerSent {
		if !w.statusSet {
			w.status = http.StatusOK
			w.statusSet = true
		}
		if !w.bodyStarted {
			// Probe negotiation for an empty response so an explicit refusal of
			// every coding, including identity, still receives 406. For an
			// otherwise acceptable empty response, preserve the historical
			// behavior of sending it without a compression header.
			_ = w.selectAlgorithm(w.status)
			if !w.negotiationFailed && w.algorithm != AlgorithmIdentity && !IdentityAcceptable(strings.Join(w.request.Header.Values("Accept-Encoding"), ",")) {
				w.negotiationFailed = true
			}
			if w.negotiationFailed {
				w.sendNotAcceptable()
				return w.encoderError
			}
			w.algorithm = AlgorithmIdentity
			w.ResponseWriter.WriteHeader(w.status)
			w.headerSent = true
		} else if err := w.sendHeader(); err != nil {
			return err
		}
	}
	if len(w.pending) > 0 && !w.negotiationFailed {
		if _, err := w.writeBody(w.pending); err != nil && w.encoderError == nil {
			w.encoderError = err
		}
		w.pending = nil
	}
	if w.encoder == nil {
		return w.encoderError
	}
	if err := w.encoder.Close(); err != nil && w.encoderError == nil {
		w.encoderError = err
	}
	return w.encoderError
}

func (w *HTTPResponseWriter) Flush() {
	_ = w.FlushError()
}

func (w *HTTPResponseWriter) FlushError() error {
	if !w.statusSet {
		w.status = http.StatusOK
		w.statusSet = true
	}
	w.bodyStarted = true
	w.forceCompression = true
	if err := w.sendHeader(); err != nil {
		return err
	}
	if w.negotiationFailed {
		return nil
	}
	if w.algorithm != AlgorithmIdentity && w.encoder == nil {
		encoder, err := NewEncoder(w.ResponseWriter, w.algorithm, w.level)
		if err != nil {
			w.encoderError = err
			return err
		}
		w.encoder = encoder
	}
	if len(w.pending) > 0 {
		if _, err := w.writeBody(w.pending); err != nil {
			return err
		}
		w.pending = nil
	}
	if w.encoder != nil {
		if flusher, ok := w.encoder.(EncoderFlusher); ok {
			if err := flusher.Flush(); err != nil {
				w.encoderError = err
				return err
			}
		}
	}
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		return nil
	}
	flusher.Flush()
	return nil
}

func (w *HTTPResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *HTTPResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	conn, readWriter, err := hijacker.Hijack()
	if err == nil {
		// The connection is owned by the caller after hijacking. The deferred
		// Close in the reverse proxy must not write a late HTTP status line.
		w.closed = true
		w.headerSent = true
		w.algorithm = AlgorithmIdentity
	}
	return conn, readWriter, err
}

func (w *HTTPResponseWriter) Push(target string, options *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, options)
}

func (w *HTTPResponseWriter) sendHeader() error {
	if w.headerSent {
		return w.encoderError
	}
	if !w.statusSet {
		w.status = http.StatusOK
		w.statusSet = true
	}
	if !w.negotiationFailed {
		w.algorithm = w.selectAlgorithm(w.status)
	}
	if w.negotiationFailed {
		w.sendNotAcceptable()
		return w.encoderError
	}
	if w.algorithm != AlgorithmIdentity {
		w.Header().Del("Content-Length")
		w.Header().Del("Content-MD5")
		// A strong validator identifies the selected representation. The
		// encoded bytes differ from the upstream identity representation, so
		// retaining its ETag would make conditional requests ambiguous.
		w.Header().Del("ETag")
		w.Header().Set("Content-Encoding", string(w.algorithm))
	}
	w.ResponseWriter.WriteHeader(w.status)
	w.headerSent = true
	return nil
}

func (w *HTTPResponseWriter) writeBody(p []byte) (int, error) {
	if w.algorithm == AlgorithmIdentity {
		return w.ResponseWriter.Write(p)
	}
	if w.encoder == nil {
		encoder, err := NewEncoder(w.ResponseWriter, w.algorithm, w.level)
		if err != nil {
			w.encoderError = err
			return 0, err
		}
		w.encoder = encoder
	}
	return w.encoder.Write(p)
}

func (w *HTTPResponseWriter) shouldBufferForMinimumSize() bool {
	if w.minSize <= 0 || !w.enabled || w.forceCompression || w.request == nil {
		return false
	}
	if w.statusSet && ((w.status >= 100 && w.status < 200) || w.status == http.StatusNoContent || w.status == http.StatusResetContent || w.status == http.StatusNotModified) {
		return false
	}
	if strings.TrimSpace(w.Header().Get("Content-Length")) != "" {
		return false
	}
	if strings.TrimSpace(strings.Join(w.Header().Values("Content-Encoding"), ",")) != "" {
		return false
	}
	if isUpgradeRequest(w.request) || w.request.Method == http.MethodHead || w.request.Header.Get("Range") != "" || w.Header().Get("Content-Range") != "" {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(w.Header().Get("Content-Type"), ";")[0]))
	return compressibleContentType(contentType) && int64(len(w.pending)) < w.minSize
}

func (w *HTTPResponseWriter) sendNotAcceptable() {
	if w == nil || w.headerSent {
		return
	}
	w.algorithm = AlgorithmIdentity
	w.status = http.StatusNotAcceptable
	w.Header().Del("Content-Encoding")
	w.Header().Del("Content-Length")
	w.Header().Del("Content-MD5")
	w.Header().Del("ETag")
	w.Header().Del("Content-Range")
	AppendVaryAcceptEncoding(w.Header(), "Accept-Encoding")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.ResponseWriter.WriteHeader(w.status)
	w.headerSent = true
	_, _ = io.WriteString(w.ResponseWriter, "not acceptable\n")
}

func (w *HTTPResponseWriter) selectAlgorithm(status int) Algorithm {
	if w.request == nil {
		return AlgorithmIdentity
	}
	request := w.request
	if (status >= 100 && status < 200) || status == http.StatusNoContent || status == http.StatusResetContent || status == http.StatusNotModified {
		return AlgorithmIdentity
	}
	contentEncoding := strings.TrimSpace(strings.Join(w.Header().Values("Content-Encoding"), ","))
	if contentEncoding != "" && !strings.EqualFold(contentEncoding, string(AlgorithmIdentity)) {
		if !ContentEncodingAcceptableValues(contentEncoding, request.Header.Values("Accept-Encoding")) {
			w.negotiationFailed = true
			return AlgorithmIdentity
		}
		// A response that keeps an upstream content coding still varies with
		// the client's Accept-Encoding contract, even though this layer does
		// not add another coding.
		AppendVaryAcceptEncoding(w.Header(), "Accept-Encoding")
		return AlgorithmIdentity
	}
	if request.Method == http.MethodHead || status == http.StatusSwitchingProtocols {
		return AlgorithmIdentity
	}
	if isUpgradeRequest(request) {
		return AlgorithmIdentity
	}
	acceptHeader := strings.Join(request.Header.Values("Accept-Encoding"), ",")
	algorithm, acceptable := SelectEncodingWithAllowed(acceptHeader, w.allowedAlgorithms)
	identityAllowed := IdentityAcceptable(acceptHeader)
	if !acceptable {
		w.negotiationFailed = true
		return AlgorithmIdentity
	}
	if !w.enabled {
		if !identityAllowed {
			w.negotiationFailed = true
		}
		return AlgorithmIdentity
	}
	if request.Header.Get("Range") != "" || w.Header().Get("Content-Range") != "" {
		if !identityAllowed {
			w.negotiationFailed = true
		}
		return AlgorithmIdentity
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(w.Header().Get("Content-Type"), ";")[0]))
	if !compressibleContentType(contentType) {
		if !identityAllowed {
			w.negotiationFailed = true
		}
		return AlgorithmIdentity
	}
	AppendVaryAcceptEncoding(w.Header(), "Accept-Encoding")
	if length, err := strconv.ParseInt(strings.TrimSpace(w.Header().Get("Content-Length")), 10, 64); err == nil && length >= 0 {
		if length == 0 || length < w.minSize {
			if !identityAllowed {
				w.negotiationFailed = true
			}
			return AlgorithmIdentity
		}
	} else if !w.forceCompression && w.minSize > 0 && len(w.pending) > 0 && int64(len(w.pending)) < w.minSize {
		if !identityAllowed {
			w.negotiationFailed = true
		}
		return AlgorithmIdentity
	}
	return algorithm
}

func isUpgradeRequest(request *http.Request) bool {
	if request == nil {
		return false
	}
	if strings.TrimSpace(request.Header.Get("Upgrade")) != "" {
		return true
	}
	return request.Method == http.MethodConnect && strings.EqualFold(strings.TrimSpace(request.Proto), "websocket")
}

func compressibleContentType(contentType string) bool {
	if contentType == "" || contentType == "text/event-stream" || strings.HasPrefix(contentType, "multipart/") {
		return false
	}
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	switch contentType {
	case "application/json",
		"application/ld+json",
		"application/javascript",
		"application/x-javascript",
		"application/css",
		"text/css",
		"application/xml",
		"application/rss+xml",
		"application/atom+xml",
		"application/xhtml+xml",
		"application/wasm",
		"application/dns-message",
		"image/svg+xml",
		"application/x-www-form-urlencoded":
		return true
	default:
		return false
	}
}

// RequestBodyDecoder prepares a request body for an upstream server and
// removes Content-Encoding so the upstream receives the decoded entity.
func RequestBodyDecoder(request *http.Request, maxDecodedBytes int64) (bool, error) {
	if request == nil || request.Body == nil {
		return false, nil
	}
	header := strings.TrimSpace(strings.Join(request.Header.Values("Content-Encoding"), ","))
	if header == "" || strings.EqualFold(header, string(AlgorithmIdentity)) {
		request.Header.Del("Content-Encoding")
		return false, nil
	}
	decoded, err := NewDecoder(request.Body, header, maxDecodedBytes)
	if err != nil {
		return false, err
	}
	request.Body = decoded
	request.GetBody = nil
	request.Header.Del("Content-Encoding")
	request.Header.Del("Content-Length")
	request.ContentLength = -1
	request.TransferEncoding = nil
	return true, nil
}
