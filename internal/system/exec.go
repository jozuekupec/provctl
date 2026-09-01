package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

var ErrBinaryNotAllowed = errors.New("binary is not allowlisted")

// ExecCommander is the production command runner. It accepts only absolute,
// allowlisted binary paths and never starts a shell.
type ExecCommander struct{}

func (ExecCommander) Run(ctx context.Context, name string, args ...string) (Result, error) {
	return run(ctx, nil, name, args...)
}

func (ExecCommander) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) (Result, error) {
	return run(ctx, stdin, name, args...)
}

func (ExecCommander) RunToFile(ctx context.Context, path string, mode os.FileMode, name string, args ...string) (Result, error) {
	if !IsAllowedBinary(name) {
		return Result{}, fmt.Errorf("%w: %s", ErrBinaryNotAllowed, name)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return Result{}, fmt.Errorf("create command output %q: %w", path, err)
	}
	defer file.Close()
	started := time.Now()
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = file
	stderr := &limitedBuffer{}
	command.Stderr = stderr
	err = command.Run()
	result := Result{Stderr: stderr.String(), Duration: time.Since(started)}
	if exitError := new(exec.ExitError); errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
	}
	return result, err
}

func run(ctx context.Context, stdin io.Reader, name string, args ...string) (Result, error) {
	if !IsAllowedBinary(name) {
		return Result{}, fmt.Errorf("%w: %s", ErrBinaryNotAllowed, name)
	}
	started := time.Now()
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = stdin
	stdout, stderr := &limitedBuffer{}, &limitedBuffer{}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(started)}
	if exitError := new(exec.ExitError); errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
	}
	return result, err
}

var phpFPMBinary = regexp.MustCompile(`^/usr/sbin/php-fpm[0-9]+\.[0-9]+$`)

var allowedBinaries = map[string]struct{}{
	"/usr/sbin/apachectl": {}, "/usr/sbin/apache2ctl": {}, "/usr/bin/systemctl": {},
	"/usr/sbin/useradd": {}, "/usr/sbin/usermod": {}, "/usr/sbin/userdel": {},
	"/usr/sbin/chpasswd": {}, "/usr/bin/crontab": {}, "/usr/bin/certbot": {},
	"/usr/bin/mysql": {}, "/usr/bin/mysqldump": {}, "/usr/bin/tar": {},
	"/usr/bin/zstd": {}, "/usr/bin/du": {}, "/usr/bin/openssl": {},
	"/usr/bin/dig": {}, "/usr/bin/getent": {}, "/usr/bin/ssh-keygen": {},
}

func IsAllowedBinary(name string) bool {
	if !filepath.IsAbs(name) {
		return false
	}
	if phpFPMBinary.MatchString(name) {
		return true
	}
	_, allowed := allowedBinaries[name]
	return allowed
}

// limitedBuffer avoids retaining unbounded output from system commands.
type limitedBuffer struct{ data []byte }

const maxCommandOutput = 1 << 20

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	remaining := maxCommandOutput - len(buffer.data)
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		buffer.data = append(buffer.data, data...)
	}
	return originalLength, nil
}

func (buffer *limitedBuffer) String() string { return string(buffer.data) }
