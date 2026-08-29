package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"provctl/internal/system"
)

// DefaultCertificate creates the certificate used only by the catch-all vhost.
type DefaultCertificate struct {
	FS          system.FS
	Commands    system.Commander
	Directory   string
	Certificate string
	Key         string
}

func (certificate DefaultCertificate) Ensure(ctx context.Context) (func(context.Context) error, bool, error) {
	if certificate.FS == nil || certificate.Commands == nil || certificate.Directory == "" || certificate.Certificate == "" || certificate.Key == "" {
		return nil, false, errors.New("default certificate requires filesystem, commander, directory, certificate, and key")
	}
	certificateExists, err := exists(certificate.FS, certificate.Certificate)
	if err != nil {
		return nil, false, err
	}
	keyExists, err := exists(certificate.FS, certificate.Key)
	if err != nil {
		return nil, false, err
	}
	if certificateExists && keyExists {
		return func(context.Context) error { return nil }, false, nil
	}
	if certificateExists || keyExists {
		return nil, false, errors.New("refuse to replace incomplete default TLS certificate")
	}
	if err := certificate.FS.MkdirAll(certificate.Directory, 0o700); err != nil {
		return nil, false, fmt.Errorf("create default TLS directory: %w", err)
	}
	if err := certificate.FS.Chmod(certificate.Directory, 0o700); err != nil {
		return nil, false, fmt.Errorf("secure default TLS directory: %w", err)
	}
	_, err = certificate.Commands.Run(ctx, "/usr/bin/openssl", "req", "-x509", "-new", "-nodes", "-newkey", "rsa:2048", "-keyout", certificate.Key, "-out", certificate.Certificate, "-days", "3650", "-subj", "/CN=provctl-default.invalid")
	if err != nil {
		return nil, false, fmt.Errorf("generate default TLS certificate: %w", err)
	}
	if err := certificate.FS.Chmod(certificate.Key, 0o600); err != nil {
		return nil, false, fmt.Errorf("secure default TLS key: %w", err)
	}
	if _, err := certificate.FS.Stat(certificate.Certificate); err != nil {
		return nil, false, fmt.Errorf("inspect generated certificate: %w", err)
	}
	undo := func(context.Context) error {
		var failures []error
		for _, path := range []string{certificate.Certificate, certificate.Key} {
			if err := certificate.FS.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
			}
		}
		return errors.Join(failures...)
	}
	return undo, true, nil
}

func exists(fs system.FS, path string) (bool, error) {
	_, err := fs.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("inspect %q: %w", path, err)
}

func defaultCertificatePaths(directory string) (string, string) {
	return filepath.Join(directory, "certificate.pem"), filepath.Join(directory, "private-key.pem")
}
