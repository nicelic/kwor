package service

import (
	"errors"
	"net/http"
	"strings"

	compressionalgorithm "github.com/alireza0/s-ui/compression/Compression-algorithm"
)

const reverseProxyCompressionLevel = compressionalgorithm.DefaultLevel

func reverseProxyPrepareRequestCompression(request *http.Request, maxDecodedBytes int64) (bool, error) {
	return compressionalgorithm.RequestBodyDecoder(request, maxDecodedBytes)
}

func reverseProxyDecodeUpstreamResponse(resp *http.Response, maxDecodedBytes int64) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	if (resp.StatusCode >= 100 && resp.StatusCode < 200) || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusResetContent || resp.StatusCode == http.StatusNotModified {
		return nil
	}
	if resp.Request != nil && resp.Request.Method == http.MethodHead {
		return nil
	}
	if resp.Request != nil && resp.Request.Header.Get("Range") != "" {
		return nil
	}
	if resp.Header.Get("Content-Range") != "" {
		return nil
	}
	header := strings.TrimSpace(strings.Join(resp.Header.Values("Content-Encoding"), ","))
	if header == "" || strings.EqualFold(header, string(compressionalgorithm.AlgorithmIdentity)) {
		resp.Header.Del("Content-Encoding")
		return nil
	}
	decoded, err := compressionalgorithm.NewDecoder(resp.Body, header, maxDecodedBytes)
	if err != nil {
		// Unknown upstream extensions are passed through unchanged. The response
		// writer later checks the client's Accept-Encoding contract; known codec
		// initialization failures must fail the proxy response.
		if errors.Is(err, compressionalgorithm.ErrUnsupportedEncoding) {
			return nil
		}
		return err
	}
	resp.Body = decoded
	resp.ContentLength = -1
	resp.TransferEncoding = nil
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	resp.Header.Del("ETag")
	resp.Header.Del("Content-MD5")
	return nil
}
