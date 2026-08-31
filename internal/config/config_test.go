package config

import (
	"strings"
	"testing"
)

func TestDecode_DefaultConfiguration(t *testing.T) {
	cfg, err := Decode(strings.NewReader(defaultConfig))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if cfg.Meta.ConfigVersion != CurrentVersion {
		t.Errorf("config version = %d, want %d", cfg.Meta.ConfigVersion, CurrentVersion)
	}
	if cfg.PHP.DefaultVersion != "" {
		t.Errorf("default PHP version = %q, want automatic selection", cfg.PHP.DefaultVersion)
	}
}

func TestDecode_MissingConfigVersionDefaultsToOne(t *testing.T) {
	input := strings.Replace(defaultConfig, "config_version = 1\n", "", 1)
	cfg, err := Decode(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if cfg.Meta.ConfigVersion != 1 {
		t.Errorf("config version = %d, want 1", cfg.Meta.ConfigVersion)
	}
}

func TestDecode_SSLServer(t *testing.T) {
	valid := strings.Replace(defaultConfig, "server = \"\"", "server = \"https://pebble.test/dir\"", 1)
	if _, err := Decode(strings.NewReader(valid)); err != nil {
		t.Fatalf("Decode(valid SSL server) error = %v", err)
	}
	invalid := strings.Replace(defaultConfig, "server = \"\"", "server = \"http://pebble.test/dir\"", 1)
	if _, err := Decode(strings.NewReader(invalid)); err == nil {
		t.Fatal("Decode(invalid SSL server) error = nil, want error")
	}
}

const defaultConfig = `
[meta]
config_version = 1
[paths]
vhosts = "/var/www/vhosts"
backups = "/var/backups/provctl"
acme_challenge = "/var/lib/provctl/acme-challenge"
[apache]
service = "apache2"
sites_available = "/etc/apache2/sites-available"
sites_enabled = "/etc/apache2/sites-enabled"
proxy_timeout = 60
allowed_proxy_hosts = ["127.0.0.1"]
[php]
default_version = ""
max_children = 10
memory_limit = "256M"
upload_max = "64M"
max_exec_time = 60
[mariadb]
enabled = true
host = "localhost"
[users]
uid_min = 5000
uid_max = 59999
shell = "/bin/bash"
[ssl]
email = ""
staging = false
server = ""
[logs]
retention_days = 14
compress = true
[limits]
lock_timeout_seconds = 30
`
