package compressionalgorithm

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoundTripAllAlgorithms(t *testing.T) {
	input := []byte(strings.Repeat("kwor reverse proxy compression test payload ", 256))
	for _, algorithm := range Priority {
		t.Run(string(algorithm), func(t *testing.T) {
			var compressed bytes.Buffer
			writer, err := NewEncoder(&compressed, algorithm, DefaultLevelFor(algorithm))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Write(input); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			reader, err := NewDecoder(io.NopCloser(bytes.NewReader(compressed.Bytes())), string(algorithm), 0)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
			if !bytes.Equal(decoded, input) {
				t.Fatalf("decoded payload differs: got %d bytes, want %d", len(decoded), len(input))
			}
		})
	}
}

func TestDefaultCompressionLevels(t *testing.T) {
	if DefaultLevel != 8 {
		t.Fatalf("zstd DefaultLevel = %d, want 8", DefaultLevel)
	}
	if DefaultLevelFor(AlgorithmZstd) != 8 {
		t.Fatalf("zstd default = %d, want 8", DefaultLevelFor(AlgorithmZstd))
	}
	for _, algorithm := range []Algorithm{AlgorithmS2, AlgorithmBrotli, AlgorithmDeflate, AlgorithmGzip} {
		if got := DefaultLevelFor(algorithm); got != 6 {
			t.Fatalf("%s default = %d, want 6", algorithm, got)
		}
	}
}

func TestHTTPCompressionWindowPolicies(t *testing.T) {
	if httpZstdMaximumWindowBytes != 32<<20 {
		t.Fatalf("zstd HTTP window = %d, want 32 MiB rounded down from 36 MiB", httpZstdMaximumWindowBytes)
	}
	if httpBrotliMaximumWindowBits != 24 {
		t.Fatalf("Brotli lgwin = %d, want standard maximum 24", httpBrotliMaximumWindowBits)
	}
}

func TestEncoderWindowBytesForContentLengthIsBoundedAndDynamic(t *testing.T) {
	if got := encoderWindowBytesForContentLength(AlgorithmZstd, 900); got != 1<<10 {
		t.Fatalf("small zstd window = %d, want 1 KiB", got)
	}
	if got := encoderWindowBytesForContentLength(AlgorithmZstd, 9<<20); got != 16<<20 {
		t.Fatalf("zstd 9 MiB window = %d, want 16 MiB", got)
	}
	if got := encoderWindowBytesForContentLength(AlgorithmZstd, 36<<20); got != 32<<20 {
		t.Fatalf("zstd 36 MiB window = %d, want 32 MiB cap", got)
	}
	if got := encoderWindowBytesForContentLength(AlgorithmBrotli, 36<<20); got != 16<<20 {
		t.Fatalf("Brotli 36 MiB window = %d, want 16 MiB cap", got)
	}
	if got := encoderWindowBytesForContentLength(AlgorithmDeflate, 36<<20); got != 0 {
		t.Fatalf("Deflate window = %d, want codec default/fixed window", got)
	}
}

func TestSelectEncodingUsesSupportAndConfiguredPriority(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   Algorithm
		ok     bool
	}{
		{name: "priority", header: "gzip, deflate, br, zstd", want: AlgorithmZstd, ok: true},
		{name: "positive q does not change priority", header: "zstd;q=0.00001, br;q=1, gzip;q=0.8", want: AlgorithmZstd, ok: true},
		{name: "zero q refuses the algorithm", header: "zstd;q=0, br;q=0.00001, gzip;q=0.8", want: AlgorithmBrotli, ok: true},
		{name: "wildcard", header: "*;q=1", want: AlgorithmZstd, ok: true},
		{name: "identity", header: "gzip;q=0, br;q=0", want: AlgorithmIdentity, ok: true},
		{name: "wildcard rejects identity", header: "*;q=0", want: AlgorithmIdentity, ok: false},
		{name: "none", header: "*;q=0, identity;q=0", want: AlgorithmIdentity, ok: false},
		{name: "explicit extension", header: "s2, gzip;q=0.5", want: AlgorithmS2, ok: true},
		{name: "invalid nan quality ignored", header: "zstd;q=NaN, br;q=1", want: AlgorithmBrotli, ok: true},
		{name: "invalid exponent ignored", header: "zstd;q=1e0, br;q=0", want: AlgorithmIdentity, ok: true},
		{name: "positive extra precision is accepted", header: "zstd;q=0.00001, br;q=0", want: AlgorithmZstd, ok: true},
		{name: "valid trailing decimal point", header: "zstd;q=1., br;q=0", want: AlgorithmZstd, ok: true},
		{name: "duplicate restrictive quality", header: "zstd;q=1, zstd;q=0", want: AlgorithmIdentity, ok: true},
		{name: "invalid missing q value ignored", header: "zstd;q, br;q=1", want: AlgorithmBrotli, ok: true},
		{name: "invalid extension parameter ignored", header: "zstd;level=1, br;q=1", want: AlgorithmBrotli, ok: true},
		{name: "invalid qvalue whitespace ignored", header: "zstd;q= 1, br;q=1", want: AlgorithmBrotli, ok: true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, ok := SelectEncoding(item.header)
			if got != item.want || ok != item.ok {
				t.Fatalf("SelectEncoding(%q) = %q, %v; want %q, %v", item.header, got, ok, item.want, item.ok)
			}
		})
	}
}

func TestContentEncodingAcceptabilityFollowsWildcardAndExplicitValues(t *testing.T) {
	cases := []struct {
		name     string
		encoding string
		accept   string
		want     bool
	}{
		{name: "absent accept header", encoding: "x-private", accept: "", want: true},
		{name: "explicit accepted", encoding: "x-private", accept: "x-private", want: true},
		{name: "explicit rejected", encoding: "x-private", accept: "x-private;q=0", want: false},
		{name: "wildcard accepted", encoding: "x-private", accept: "*", want: true},
		{name: "wildcard rejected", encoding: "x-private", accept: "*;q=0", want: false},
		{name: "stacked all accepted", encoding: "gzip, x-private", accept: "gzip, x-private", want: true},
		{name: "stacked one missing", encoding: "gzip, x-private", accept: "gzip", want: false},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := ContentEncodingAcceptable(item.encoding, item.accept); got != item.want {
				t.Fatalf("ContentEncodingAcceptable(%q, %q) = %v, want %v", item.encoding, item.accept, got, item.want)
			}
		})
	}
}

func TestContentEncodingAcceptabilityDistinguishesAbsentAndEmptyHeader(t *testing.T) {
	if !ContentEncodingAcceptableValues("x-private", nil) {
		t.Fatal("absent Accept-Encoding must accept an existing coding")
	}
	if ContentEncodingAcceptableValues("x-private", []string{""}) {
		t.Fatal("explicit empty Accept-Encoding must reject an existing coding")
	}
}

func TestWildcardUsesConfiguredPriority(t *testing.T) {
	got, ok := SelectEncoding("*")
	if !ok || got != AlgorithmZstd {
		t.Fatalf("SelectEncoding(\"*\") = %q, %v; want zstd, true", got, ok)
	}
	if got, ok := SelectEncoding("s2;q=0.00001, gzip;q=1"); !ok || got != AlgorithmS2 {
		t.Fatalf("explicit s2 offer = %q, %v; want s2, true", got, ok)
	}
}

func TestUpstreamAcceptEncodingIncludesAllSupportedAlgorithms(t *testing.T) {
	header := UpstreamAcceptEncoding()
	want := "zstd;q=1.000, s2;q=0.999, snappy;q=0.998, br;q=0.997, deflate;q=0.996, gzip;q=0.995"
	if header != want {
		t.Fatalf("UpstreamAcceptEncoding() = %q, want %q", header, want)
	}
	lastIndex := -1
	for _, token := range []string{"zstd", "s2", "snappy", "br", "deflate", "gzip"} {
		if !strings.Contains(header, token) {
			t.Fatalf("upstream advertisement missing %q: %q", token, header)
		}
		index := strings.Index(header, token)
		if index <= lastIndex {
			t.Fatalf("upstream advertisement order = %q, token %q appeared at %d after %d", header, token, index, lastIndex)
		}
		lastIndex = index
	}
}

func TestNegotiationDoesNotDependOnMutablePrioritySnapshot(t *testing.T) {
	original := Priority
	Priority = [...]Algorithm{
		AlgorithmGzip,
		AlgorithmDeflate,
		AlgorithmBrotli,
		AlgorithmSnappy,
		AlgorithmS2,
		AlgorithmZstd,
	}
	t.Cleanup(func() { Priority = original })

	if got, ok := SelectEncoding("gzip, deflate, br, snappy, s2, zstd"); !ok || got != AlgorithmZstd {
		t.Fatalf("SelectEncoding used mutable Priority snapshot: got %q, %v; want zstd, true", got, ok)
	}
	want := "zstd;q=1.000, s2;q=0.999, snappy;q=0.998, br;q=0.997, deflate;q=0.996, gzip;q=0.995"
	if got := UpstreamAcceptEncoding(); got != want {
		t.Fatalf("UpstreamAcceptEncoding used mutable Priority snapshot: got %q, want %q", got, want)
	}
}

func TestAppendVaryAcceptEncodingKeepsWildcardVary(t *testing.T) {
	header := make(http.Header)
	header.Set("Vary", "Origin, *")
	AppendVaryAcceptEncoding(header, "Accept-Encoding")
	if got := header.Get("Vary"); got != "Origin, *" {
		t.Fatalf("Vary = %q, want wildcard header unchanged", got)
	}
}

func TestDecoderLimitAllowsExactBoundaryAndRejectsOverflow(t *testing.T) {
	input := []byte("0123456789")
	reader, err := NewDecoder(io.NopCloser(bytes.NewReader(input)), "identity", int64(len(input)))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("exact boundary read failed: %v", err)
	}
	_ = reader.Close()
	if !bytes.Equal(decoded, input) {
		t.Fatalf("exact boundary decoded %q, want %q", decoded, input)
	}

	reader, err = NewDecoder(io.NopCloser(bytes.NewReader(input)), "identity", int64(len(input)-1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(reader)
	_ = reader.Close()
	if !errors.Is(err, ErrDecodedSizeLimit) {
		t.Fatalf("overflow read error = %v, want ErrDecodedSizeLimit", err)
	}
}

func TestHTTPResponseWriterCompressesAndSetsVary(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "gzip, br")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	recorder.Header().Set("ETag", `"identity-representation"`)
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 0, Level: DefaultLevel})
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != string(AlgorithmBrotli) {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
	if !strings.Contains(recorder.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("Vary header missing Accept-Encoding: %q", recorder.Header().Get("Vary"))
	}
	if got := recorder.Header().Get("ETag"); got != "" {
		t.Fatalf("ETag = %q, want cleared after content encoding", got)
	}
	decoded, err := NewDecoder(io.NopCloser(bytes.NewReader(recorder.Body.Bytes())), recorder.Header().Get("Content-Encoding"), 0)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(decoded)
	_ = decoded.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("decoded response = %q", body)
	}
}

func TestHTTPResponseWriterEmptyAllowedAlgorithmsUsesIdentity(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "zstd, gzip")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{
		Request:           request,
		Enabled:           true,
		MinSize:           0,
		Level:             DefaultLevel,
		AllowedAlgorithms: []Algorithm{},
	})
	body := strings.Repeat(`{"ok":true}`, 32)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want identity", got)
	}
	if got := recorder.Body.String(); got != body {
		t.Fatalf("body = %q, want original body", got)
	}
}

func TestHTTPResponseWriterIgnoresPositiveQMagnitude(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "zstd;q=0.00001, br;q=1, gzip;q=0.8")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 0, Level: DefaultLevel})
	if _, err := writer.Write([]byte(strings.Repeat(`{"ok":true}`, 32))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != string(AlgorithmZstd) {
		t.Fatalf("Content-Encoding = %q, want zstd", got)
	}
}

func TestHTTPResponseWriterBuffersUnknownLengthSmallResponse(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "gzip, br")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 256, Level: DefaultLevel})
	if _, err := writer.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 {
		// The underlying recorder must not receive a status before the buffered
		// body is classified at Close.
		t.Fatalf("status before close = %d, want 200 recorder default", recorder.Code)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("small response Content-Encoding = %q, want empty", got)
	}
	if got := recorder.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body = %q, want original body", got)
	}
}

func TestHTTPResponseWriterFlushesBufferedResponseWithCompression(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 256, Level: DefaultLevel})
	if _, err := writer.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.FlushError(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != string(AlgorithmGzip) {
		t.Fatalf("flushed response Content-Encoding = %q, want gzip", got)
	}
}

func TestHTTPResponseWriterCombinesRepeatedAcceptEncodingHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header["Accept-Encoding"] = []string{"gzip", "br"}
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 0, Level: DefaultLevel})
	if _, err := writer.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != string(AlgorithmBrotli) {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
}

func TestHTTPResponseWriterDoesNotEmitCompressionForEmptyResponse(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "zstd, br, gzip")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 0, Level: DefaultLevel})
	writer.WriteHeader(http.StatusOK)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("empty response Content-Encoding = %q, want empty", got)
	}
	if got := recorder.Code; got != http.StatusOK {
		t.Fatalf("empty response status = %d, want %d", got, http.StatusOK)
	}
}

func TestHTTPResponseWriterRejectsUnknownLengthEmptyResponseWhenIdentityForbidden(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "gzip, identity;q=0")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 0, Level: DefaultLevel})
	writer.WriteHeader(http.StatusOK)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusNotAcceptable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotAcceptable)
	}
}

func TestHTTPResponseWriterRejectsWhenIdentityAndAllCodingsAreForbidden(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "*;q=0")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 0, Level: DefaultLevel})
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusNotAcceptable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotAcceptable)
	}
	if got := recorder.Body.String(); got != "not acceptable\n" {
		t.Fatalf("body = %q, want not acceptable response", got)
	}
	if !strings.Contains(recorder.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("406 response Vary header missing Accept-Encoding: %q", recorder.Header().Get("Vary"))
	}
}

func TestHTTPResponseWriterRejectsIdentityWhenResponseCannotBeCompressed(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		contentLen  string
	}{
		{name: "non-compressible type", contentType: "image/png"},
		{name: "small body", contentType: "application/json", contentLen: "1"},
		{name: "missing content type", contentLen: "1"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
			request.Header.Set("Accept-Encoding", "identity;q=0")
			recorder := httptest.NewRecorder()
			if item.contentType != "" {
				recorder.Header().Set("Content-Type", item.contentType)
			}
			if item.contentLen != "" {
				recorder.Header().Set("Content-Length", item.contentLen)
			}
			writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: DefaultMinimumResponseSize, Level: DefaultLevel})
			writer.WriteHeader(http.StatusOK)
			if _, err := writer.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusNotAcceptable {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotAcceptable)
			}
		})
	}
}

func TestHTTPResponseWriterRejectsIdentityWhenCompressionIsDisabled(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "identity;q=0")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: false, MinSize: 0, Level: DefaultLevel})
	if _, err := writer.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusNotAcceptable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotAcceptable)
	}
}

func TestHTTPResponseWriterRejectsUnknownEncodingNotAcceptedByClient(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/octet-stream")
	recorder.Header().Set("Content-Encoding", "x-private")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 0, Level: DefaultLevel})
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte("encoded")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusNotAcceptable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotAcceptable)
	}
}

func TestHTTPResponseWriterPreservesAcceptedUnknownEncodingAndVary(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Accept-Encoding", "x-private")
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/octet-stream")
	recorder.Header().Set("Content-Encoding", "x-private")
	writer := NewHTTPResponseWriter(recorder, HTTPResponseOptions{Request: request, Enabled: true, MinSize: 0, Level: DefaultLevel})
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte("encoded")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "x-private" {
		t.Fatalf("Content-Encoding = %q, want x-private", got)
	}
	if got := recorder.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
}
