package service

import (
	"bytes"
	"testing"

	compressionalgorithm "github.com/alireza0/s-ui/compression/Compression-algorithm"
)

func TestDecodeRuleSetProbeBodyDecodesStackedContentEncoding(t *testing.T) {
	input := []byte("stacked rule-set payload")
	var zstdEncoded bytes.Buffer
	zstdWriter, err := compressionalgorithm.NewEncoder(&zstdEncoded, compressionalgorithm.AlgorithmZstd, compressionalgorithm.DefaultLevel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zstdWriter.Write(input); err != nil {
		t.Fatal(err)
	}
	if err := zstdWriter.Close(); err != nil {
		t.Fatal(err)
	}

	var gzipEncoded bytes.Buffer
	gzipWriter, err := compressionalgorithm.NewEncoder(&gzipEncoded, compressionalgorithm.AlgorithmGzip, compressionalgorithm.DefaultLevel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gzipWriter.Write(zstdEncoded.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeRuleSetProbeBody(gzipEncoded.Bytes(), "zstd, gzip")
	if err != nil {
		t.Fatalf("decode stacked response failed: %v", err)
	}
	if !bytes.Equal(decoded, input) {
		t.Fatalf("decoded body = %q, want %q", decoded, input)
	}
}

func TestDecodeRuleSetProbeBodyRejectsUnsupportedContentEncoding(t *testing.T) {
	if _, err := decodeRuleSetProbeBody([]byte("payload"), "made-up"); err == nil {
		t.Fatal("unsupported content encoding should fail")
	}
}

func TestDecodeRuleSetProbeBodyRejectsUnrequestedContentEncoding(t *testing.T) {
	if _, err := decodeRuleSetProbeBody([]byte("payload"), "br"); err == nil {
		t.Fatal("content encoding not advertised by the probe should fail")
	}
}
