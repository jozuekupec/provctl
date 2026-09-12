// Package config loads and validates provctl's TOML configuration.
package config

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const CurrentVersion = 1

type Config struct {
	Meta    Meta    `toml:"meta"`
	Paths   Paths   `toml:"paths"`
	Apache  Apache  `toml:"apache"`
	PHP     PHP     `toml:"php"`
	MariaDB MariaDB `toml:"mariadb"`
	Users   Users   `toml:"users"`
	SSL     SSL     `toml:"ssl"`
	Logs    Logs    `toml:"logs"`
	Limits  Limits  `toml:"limits"`
}

type Meta struct {
	ConfigVersion int `toml:"config_version"`
}
type Paths struct {
	VHosts        string `toml:"vhosts"`
	Backups       string `toml:"backups"`
	ACMEChallenge string `toml:"acme_challenge"`
}
type Apache struct {
	Service           string   `toml:"service"`
	SitesAvailable    string   `toml:"sites_available"`
	SitesEnabled      string   `toml:"sites_enabled"`
	ProxyTimeout      int      `toml:"proxy_timeout"`
	AllowedProxyHosts []string `toml:"allowed_proxy_hosts"`
}
type PHP struct {
	DefaultVersion string `toml:"default_version"`
	MaxChildren    int    `toml:"max_children"`
	MemoryLimit    string `toml:"memory_limit"`
	UploadMax      string `toml:"upload_max"`
	MaxExecTime    int    `toml:"max_exec_time"`
}
type MariaDB struct {
	Enabled      bool   `toml:"enabled"`
	Host         string `toml:"host"`
	DefaultsFile string `toml:"defaults_file"`
}
type Users struct {
	UIDMin int    `toml:"uid_min"`
	UIDMax int    `toml:"uid_max"`
	Shell  string `toml:"shell"`
}
type SSL struct {
	Email   string `toml:"email"`
	Staging bool   `toml:"staging"`
	Server  string `toml:"server"`
}
type Logs struct {
	RetentionDays int  `toml:"retention_days"`
	Compress      bool `toml:"compress"`
}
type Limits struct {
	LockTimeoutSeconds int `toml:"lock_timeout_seconds"`
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()
	return Decode(file)
}

func Decode(reader io.Reader) (Config, error) {
	var cfg Config
	if _, err := toml.NewDecoder(reader).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if cfg.Meta.ConfigVersion == 0 {
		cfg.Meta.ConfigVersion = 1
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	if cfg.Meta.ConfigVersion > CurrentVersion {
		return fmt.Errorf("config version %d is newer than supported version %d", cfg.Meta.ConfigVersion, CurrentVersion)
	}
	if cfg.Meta.ConfigVersion < 1 {
		return fmt.Errorf("config version must be positive")
	}
	if cfg.Paths.VHosts == "" || cfg.Paths.Backups == "" || cfg.Paths.ACMEChallenge == "" {
		return fmt.Errorf("[paths] vhosts, backups, and acme_challenge must be set")
	}
	if cfg.Apache.Service == "" || cfg.Apache.SitesAvailable == "" || cfg.Apache.SitesEnabled == "" {
		return fmt.Errorf("[apache] service, sites_available, and sites_enabled must be set")
	}
	if cfg.Users.UIDMin > cfg.Users.UIDMax {
		return fmt.Errorf("[users] uid_min must not exceed uid_max")
	}
	if cfg.Limits.LockTimeoutSeconds <= 0 {
		return fmt.Errorf("[limits] lock_timeout_seconds must be positive")
	}
	if cfg.SSL.Server != "" {
		server, err := url.ParseRequestURI(cfg.SSL.Server)
		if err != nil || server.Scheme != "https" || server.Host == "" {
			return fmt.Errorf("[ssl] server must be an absolute HTTPS URL")
		}
	}
	return nil
}

// UpdateSSL writes the TUI-managed SSL settings without rewriting unrelated
// configuration or removing administrator comments. The rest of the document
// remains the source of truth, including an optional private ACME endpoint.
func UpdateSSL(path, email string, staging bool) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	cfg.SSL.Email, cfg.SSL.Staging = strings.TrimSpace(email), staging
	if err := cfg.Validate(); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config %q: %w", path, err)
	}
	updated, err := replaceSSLFields(string(raw), cfg.SSL.Email, cfg.SSL.Staging)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(updated), 0o640)
}

func replaceSSLFields(raw, email string, staging bool) (string, error) {
	lines := strings.Split(raw, "\n")
	start, end := -1, len(lines)
	for index, line := range lines {
		if strings.TrimSpace(line) == "[ssl]" {
			start = index
			continue
		}
		if start >= 0 && strings.HasPrefix(strings.TrimSpace(line), "[") {
			end = index
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("config %q has no [ssl] section", "document")
	}
	seenEmail, seenStaging := false, false
	for index := start + 1; index < end; index++ {
		key, indentation, ok := tomlAssignment(lines[index])
		if !ok {
			continue
		}
		switch key {
		case "email":
			lines[index], seenEmail = indentation+"email = "+strconv.Quote(email)+tomlComment(lines[index]), true
		case "staging":
			lines[index], seenStaging = indentation+"staging = "+strconv.FormatBool(staging)+tomlComment(lines[index]), true
		}
	}
	missing := make([]string, 0, 2)
	if !seenEmail {
		missing = append(missing, "email = "+strconv.Quote(email))
	}
	if !seenStaging {
		missing = append(missing, "staging = "+strconv.FormatBool(staging))
	}
	if len(missing) > 0 {
		insert := append([]string{}, lines[:end]...)
		insert = append(insert, missing...)
		lines = append(insert, lines[end:]...)
	}
	return strings.Join(lines, "\n"), nil
}

func tomlComment(line string) string {
	if index := strings.Index(line, "#"); index >= 0 {
		return " " + strings.TrimSpace(line[index:])
	}
	return ""
}

func tomlAssignment(line string) (key, indentation string, ok bool) {
	trimmed := strings.TrimSpace(line)
	before, _, found := strings.Cut(trimmed, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(before)
	if key == "" || strings.ContainsAny(key, " \t#") {
		return "", "", false
	}
	return key, line[:len(line)-len(strings.TrimLeft(line, " \t"))], true
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".provctl-*")
	if err != nil {
		return fmt.Errorf("create temporary config for %q: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(mode); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace config %q: %w", path, err)
	}
	return nil
}
