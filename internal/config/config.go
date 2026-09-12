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

// Update writes every known configuration key while preserving comments,
// ordering, and administrator-owned unknown keys. It is deliberately textual:
// TOML re-encoding would discard the operational notes in config.toml.
func Update(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config %q: %w", path, err)
	}
	updated, err := replaceConfigFields(string(raw), configAssignments(cfg))
	if err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(updated), 0o640)
}

// UpdateSSL remains the narrow programmatic API for callers that only need
// the certificate settings; the TUI uses Update for the complete form.
func UpdateSSL(path, email string, staging bool) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	cfg.SSL.Email, cfg.SSL.Staging = strings.TrimSpace(email), staging
	return Update(path, cfg)
}

func configAssignments(cfg Config) map[string]map[string]string {
	quotedList := func(values []string) string {
		items := make([]string, len(values))
		for index, value := range values {
			items[index] = strconv.Quote(value)
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	return map[string]map[string]string{
		"meta":    {"config_version": strconv.Itoa(cfg.Meta.ConfigVersion)},
		"paths":   {"vhosts": strconv.Quote(cfg.Paths.VHosts), "backups": strconv.Quote(cfg.Paths.Backups), "acme_challenge": strconv.Quote(cfg.Paths.ACMEChallenge)},
		"apache":  {"service": strconv.Quote(cfg.Apache.Service), "sites_available": strconv.Quote(cfg.Apache.SitesAvailable), "sites_enabled": strconv.Quote(cfg.Apache.SitesEnabled), "proxy_timeout": strconv.Itoa(cfg.Apache.ProxyTimeout), "allowed_proxy_hosts": quotedList(cfg.Apache.AllowedProxyHosts)},
		"php":     {"default_version": strconv.Quote(cfg.PHP.DefaultVersion), "max_children": strconv.Itoa(cfg.PHP.MaxChildren), "memory_limit": strconv.Quote(cfg.PHP.MemoryLimit), "upload_max": strconv.Quote(cfg.PHP.UploadMax), "max_exec_time": strconv.Itoa(cfg.PHP.MaxExecTime)},
		"mariadb": {"enabled": strconv.FormatBool(cfg.MariaDB.Enabled), "host": strconv.Quote(cfg.MariaDB.Host), "defaults_file": strconv.Quote(cfg.MariaDB.DefaultsFile)},
		"users":   {"uid_min": strconv.Itoa(cfg.Users.UIDMin), "uid_max": strconv.Itoa(cfg.Users.UIDMax), "shell": strconv.Quote(cfg.Users.Shell)},
		"ssl":     {"email": strconv.Quote(cfg.SSL.Email), "staging": strconv.FormatBool(cfg.SSL.Staging), "server": strconv.Quote(cfg.SSL.Server)},
		"logs":    {"retention_days": strconv.Itoa(cfg.Logs.RetentionDays), "compress": strconv.FormatBool(cfg.Logs.Compress)},
		"limits":  {"lock_timeout_seconds": strconv.Itoa(cfg.Limits.LockTimeoutSeconds)},
	}
}

func replaceConfigFields(raw string, assignments map[string]map[string]string) (string, error) {
	lines := strings.Split(raw, "\n")
	for section, fields := range assignments {
		start, end := sectionBounds(lines, section)
		if start < 0 {
			return "", fmt.Errorf("configuration has no [%s] section", section)
		}
		seen := make(map[string]bool, len(fields))
		for index := start + 1; index < end; index++ {
			key, indentation, ok := tomlAssignment(lines[index])
			if !ok || fields[key] == "" {
				continue
			}
			lines[index], seen[key] = indentation+key+" = "+fields[key]+tomlComment(lines[index]), true
		}
		missing := make([]string, 0, len(fields))
		for _, key := range configKeys(section) {
			if !seen[key] {
				missing = append(missing, key+" = "+fields[key])
			}
		}
		if len(missing) == 0 {
			continue
		}
		insert := append([]string{}, lines[:end]...)
		insert = append(insert, missing...)
		lines = append(insert, lines[end:]...)
	}
	return strings.Join(lines, "\n"), nil
}

func sectionBounds(lines []string, section string) (start, end int) {
	start, end = -1, len(lines)
	for index, line := range lines {
		if strings.TrimSpace(line) == "["+section+"]" {
			start = index
			continue
		}
		if start >= 0 && strings.HasPrefix(strings.TrimSpace(line), "[") {
			return start, index
		}
	}
	return start, end
}

func configKeys(section string) []string {
	switch section {
	case "meta":
		return []string{"config_version"}
	case "paths":
		return []string{"vhosts", "backups", "acme_challenge"}
	case "apache":
		return []string{"service", "sites_available", "sites_enabled", "proxy_timeout", "allowed_proxy_hosts"}
	case "php":
		return []string{"default_version", "max_children", "memory_limit", "upload_max", "max_exec_time"}
	case "mariadb":
		return []string{"enabled", "host", "defaults_file"}
	case "users":
		return []string{"uid_min", "uid_max", "shell"}
	case "ssl":
		return []string{"email", "staging", "server"}
	case "logs":
		return []string{"retention_days", "compress"}
	default:
		return []string{"lock_timeout_seconds"}
	}
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
