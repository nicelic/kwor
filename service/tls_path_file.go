package service

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const maxTLSPathMaterialBytes int64 = 2 * 1024 * 1024

type tlsPathFileIdentity struct {
	Size         int64
	ModifiedNano int64
}

func inspectTLSPathMaterial(path string) (tlsPathFileIdentity, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return tlsPathFileIdentity{}, fmt.Errorf("TLS material path is empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return tlsPathFileIdentity{}, err
	}
	if !info.Mode().IsRegular() {
		return tlsPathFileIdentity{}, fmt.Errorf("TLS material path is not a regular file")
	}
	if info.Size() > maxTLSPathMaterialBytes {
		return tlsPathFileIdentity{}, fmt.Errorf("TLS material exceeds the %d byte limit", maxTLSPathMaterialBytes)
	}
	return tlsPathFileIdentity{
		Size:         info.Size(),
		ModifiedNano: info.ModTime().UnixNano(),
	}, nil
}

func readTLSPathMaterial(path string) ([]byte, tlsPathFileIdentity, error) {
	identity, err := inspectTLSPathMaterial(path)
	if err != nil {
		return nil, tlsPathFileIdentity{}, err
	}

	file, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return nil, tlsPathFileIdentity{}, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxTLSPathMaterialBytes+1))
	if err != nil {
		return nil, tlsPathFileIdentity{}, err
	}
	if int64(len(data)) > maxTLSPathMaterialBytes {
		return nil, tlsPathFileIdentity{}, fmt.Errorf("TLS material exceeds the %d byte limit", maxTLSPathMaterialBytes)
	}

	return data, identity, nil
}
