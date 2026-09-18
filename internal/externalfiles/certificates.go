package externalfiles

import (
	"crypto/x509"
	"io"
	"os"
	"path/filepath"
)

// certificateRoots honors conventional CA environment variables on every OS,
// including systems where Go normally uses only the platform certificate store.
func certificateRoots() (*x509.CertPool, error) {
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
		f, err := os.Open(name)
		if err != nil {
			return false, errUnavailable
		}
		defer func() { _ = f.Close() }()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > budget {
			return false, errUnavailable
		}
		data, err := io.ReadAll(io.LimitReader(f, budget+1))
		if err != nil || int64(len(data)) > budget {
			return false, errUnavailable
		}
		budget -= int64(len(data))
		return roots.AppendCertsFromPEM(data), nil
	}
	if file != "" {
		ok, err := add(file)
		if err != nil || !ok {
			return nil, errUnavailable
		}
	}
	for _, dir := range filepath.SplitList(dirs) {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 512 {
			return nil, errUnavailable
		}
		any := false
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ok, err := add(filepath.Join(dir, entry.Name()))
			if err != nil {
				return nil, err
			}
			any = any || ok
		}
		if !any {
			return nil, errUnavailable
		}
	}
	return roots, nil
}
