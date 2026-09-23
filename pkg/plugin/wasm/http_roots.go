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

// certificateRootLoader loads deployment CA overrides within strict file and byte bounds.
type certificateRootLoader struct {
	// roots receives every accepted certificate from configured files and directories.
	roots *x509.CertPool
	// budget is the number of certificate bytes that may still be read.
	budget int64
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
	loader := certificateRootLoader{roots: roots, budget: 8 << 20}
	if file != "" {
		if ok, err := loader.addFile(file); err != nil || !ok {
			return nil, errHTTPUnavailable
		}
	}
	for _, directory := range filepath.SplitList(dirs) {
		if directory == "" {
			continue
		}
		if err := loader.addDirectory(directory); err != nil {
			return nil, err
		}
	}
	return roots, nil
}

// addFile reads one regular PEM file without exceeding the remaining certificate budget.
func (l *certificateRootLoader) addFile(name string) (bool, error) {
	input, err := os.Open(name)
	if err != nil {
		return false, errHTTPUnavailable
	}
	defer func() { _ = input.Close() }()
	info, err := input.Stat()
	if err != nil || !usableCertificateRootFile(info, l.budget) {
		return false, errHTTPUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(input, l.budget+1))
	if err != nil || int64(len(data)) > l.budget {
		return false, errHTTPUnavailable
	}
	l.budget -= int64(len(data))
	return l.roots.AppendCertsFromPEM(data), nil
}

// addDirectory loads a bounded flat certificate directory and requires at least one usable certificate.
func (l *certificateRootLoader) addDirectory(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) > 512 {
		return errHTTPUnavailable
	}
	added := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ok, err := l.addFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		added = added || ok
	}
	if !added {
		return errHTTPUnavailable
	}
	return nil
}
