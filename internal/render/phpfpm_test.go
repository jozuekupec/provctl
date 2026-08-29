package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRenderPHPFPMPool_Golden(t *testing.T) {
	pool := PHPFPMPool{Name: "acme", Home: "/var/www/vhosts/acme", Socket: "/run/php/provctl-acme.sock", MaxChildren: 10, MemoryLimit: "256M", UploadMax: "64M", MaxExecTime: 60, PhpErrorLog: "/var/log/provctl/acme/php-fpm-error.log"}
	got, err := RenderPHPFPMPool(pool)
	if err != nil {
		t.Fatalf("RenderPHPFPMPool() error = %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "php-fpm-pool.golden"))
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("render mismatch (-want +got):\n%s", diff)
	}
}

func TestRenderPHPFPMPool_ScalesWorkerValues(t *testing.T) {
	got, err := RenderPHPFPMPool(PHPFPMPool{Name: "acme", Home: "/srv/acme", Socket: "/run/php/acme.sock", MaxChildren: 1, MemoryLimit: "128M", UploadMax: "16M", MaxExecTime: 30, PhpErrorLog: "/var/log/acme.log"})
	if err != nil {
		t.Fatalf("RenderPHPFPMPool() error = %v", err)
	}
	if diff := cmp.Diff("pm.start_servers = 1\npm.min_spare_servers = 1\npm.max_spare_servers = 1", stringLinesContaining(string(got), "pm.start_servers", "pm.min_spare_servers", "pm.max_spare_servers")); diff != "" {
		t.Errorf("worker settings mismatch (-want +got):\n%s", diff)
	}
}

func stringLinesContaining(value string, prefixes ...string) string {
	var output []string
	for _, line := range strings.Split(value, "\n") {
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				output = append(output, line)
				break
			}
		}
	}
	return strings.Join(output, "\n")
}
