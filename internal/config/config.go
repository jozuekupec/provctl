// Package config loads and validates provctl's TOML configuration.
package config

import (
	"fmt"
	"io"
	"net/url"
	"os"

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
