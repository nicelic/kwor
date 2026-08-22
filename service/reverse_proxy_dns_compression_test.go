package service

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	compressionalgorithm "github.com/alireza0/s-ui/compression/Compression-algorithm"
	"github.com/miekg/dns"
)

func TestReverseProxyDNSReadDoHMessageDecodesContentEncoding(t *testing.T) {
	query := new(dns.Msg)
	query.SetQuestion("example.com.", dns.TypeA)
	wire, err := query.Pack()
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	encoder, err := compressionalgorithm.NewEncoder(&compressed, compressionalgorithm.AlgorithmZstd, compressionalgorithm.DefaultLevel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Write(wire); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "http://example.test/dns-query", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Type", "application/dns-message")
	request.Header.Set("Content-Encoding", string(compressionalgorithm.AlgorithmZstd))
	reader := httptest.NewRecorder()
	decoded, err := reverseProxyDNSReadDoHMessage(reader, request)
	if err != nil {
		t.Fatal(err)
	}
	decodedQuery := new(dns.Msg)
	if err := decodedQuery.Unpack(decoded); err != nil {
		t.Fatal(err)
	}
	if len(decodedQuery.Question) != 1 || decodedQuery.Question[0].Name != "example.com." {
		t.Fatalf("unexpected decoded query: %#v", decodedQuery)
	}
}

func TestReverseProxyDNSReadDoHMessageRejectsUnsupportedContentEncoding(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://example.test/dns-query", bytes.NewReader([]byte("dns")))
	request.Header.Set("Content-Type", "application/dns-message")
	request.Header.Set("Content-Encoding", "made-up")
	_, err := reverseProxyDNSReadDoHMessage(httptest.NewRecorder(), request)
	if !errors.Is(err, compressionalgorithm.ErrUnsupportedEncoding) {
		t.Fatalf("error = %v, want ErrUnsupportedEncoding", err)
	}
}
