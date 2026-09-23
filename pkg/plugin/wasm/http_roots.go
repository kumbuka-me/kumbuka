package wasm

import (
	"crypto/x509"
	"io"
	"os"
	"path/filepath"
)

// usableCertificateRootFile reports whether a certificate-root file is regular and fits the remaining byte budget.
func usableCertificateRootFile(info os.FileInfo, budget int64) bool {
	return info.Mode().IsRegular() && info.Size() <= budget
}

// pluginCertificateRoots honors conventional CA environment variables for plugin HTTP.
func pluginCertificateRoots() (*x509.CertPool, error) {
	file, dirs := os.Getenv("SSL_CERT_FILE"), os.Getenv("SSL_CERT_DIR")
	if file == "" && dirs == "" {
		return nil, nil
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	budget := int64(8 << 20)
	add := func(name string) (bool, error) {
		input, err := os.Open(name)
		if err != nil {
			return false, errHTTPUnavailable
		}
		defer func() { _ = input.Close() }()
		info, err := input.Stat()
		if err != nil || !usableCertificateRootFile(info, budget) {
			return false, errHTTPUnavailable
		}
		data, err := io.ReadAll(io.LimitReader(input, budget+1))
		if err != nil || int64(len(data)) > budget {
			return false, errHTTPUnavailable
		}
		budget -= int64(len(data))
		return roots.AppendCertsFromPEM(data), nil
	}
	if file != "" {
		ok, err := add(file)
		if err != nil || !ok {
			return nil, errHTTPUnavailable
		}
	}
	for _, directory := range filepath.SplitList(dirs) {
		if directory == "" {
			continue
		}
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) > 512 {
			return nil, errHTTPUnavailable
		}
		added := false
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ok, err := add(filepath.Join(directory, entry.Name()))
			if err != nil {
				return nil, err
			}
			added = added || ok
		}
		if !added {
			return nil, errHTTPUnavailable
		}
	}
	return roots, nil
}
