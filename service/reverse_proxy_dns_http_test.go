package service

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	dnsupstream "github.com/AdguardTeam/dnsproxy/upstream"
	compressionalgorithm "github.com/alireza0/s-ui/compression/Compression-algorithm"
	"github.com/miekg/dns"
)

func TestReverseProxyDNSCompressedHTTPUpstreamRoundTrip(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.Contains(request.Header.Get("Accept-Encoding"), "zstd") {
			t.Errorf("target DoH request did not advertise zstd: %q", request.Header.Get("Accept-Encoding"))
		}
		message := new(dns.Msg)
		message.SetQuestion("example.com.", dns.TypeA)
		packed, err := message.Pack()
		if err != nil {
			t.Errorf("pack target response: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/dns-message")
		writer.Header().Set("Content-Encoding", string(compressionalgorithm.AlgorithmZstd))
		encoder, err := compressionalgorithm.NewEncoder(writer, compressionalgorithm.AlgorithmZstd, compressionalgorithm.DefaultLevel)
		if err != nil {
			t.Errorf("create target response encoder: %v", err)
			return
		}
		if _, err := encoder.Write(packed); err != nil {
			t.Errorf("write target response: %v", err)
		}
		if err := encoder.Close(); err != nil {
			t.Errorf("close target response encoder: %v", err)
		}
	}))
	defer server.Close()

	upstream, err := newReverseProxyDNSCompressedHTTPUpstream(server.URL+"/dns-query", &dnsupstream.Options{
		InsecureSkipVerify: true,
		Timeout:            5 * time.Second,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()

	query := new(dns.Msg)
	query.SetQuestion("example.com.", dns.TypeA)
	response, err := upstream.Exchange(query)
	if err != nil {
		t.Fatal(err)
	}
	if response == nil || len(response.Question) != 1 || response.Question[0].Name != "example.com." {
		t.Fatalf("unexpected target DoH response: %#v", response)
	}
	if response.Id != query.Id {
		t.Fatalf("response ID changed: got %d want %d", response.Id, query.Id)
	}
}

func TestReverseProxyDNSCompressedHTTP3UpstreamRoundTrip(t *testing.T) {
	host, port := startReverseProxyTestHTTP3Server(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		message := new(dns.Msg)
		message.SetQuestion("example.com.", dns.TypeA)
		packed, err := message.Pack()
		if err != nil {
			t.Errorf("pack target response: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/dns-message")
		writer.Header().Set("Content-Encoding", string(compressionalgorithm.AlgorithmBrotli))
		encoder, err := compressionalgorithm.NewEncoder(writer, compressionalgorithm.AlgorithmBrotli, compressionalgorithm.DefaultLevelFor(compressionalgorithm.AlgorithmBrotli))
		if err != nil {
			t.Errorf("create target response encoder: %v", err)
			return
		}
		_, _ = encoder.Write(packed)
		_ = encoder.Close()
	}))

	upstream, err := newReverseProxyDNSCompressedHTTPUpstream("h3://"+host+":"+strconv.Itoa(port)+"/dns-query", &dnsupstream.Options{
		InsecureSkipVerify: true,
		Timeout:            5 * time.Second,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()

	query := new(dns.Msg)
	query.SetQuestion("example.com.", dns.TypeA)
	response, err := upstream.Exchange(query)
	if err != nil {
		t.Fatal(err)
	}
	if response == nil || len(response.Question) != 1 || response.Question[0].Name != "example.com." {
		t.Fatalf("unexpected target DoH3 response: %#v", response)
	}
}
