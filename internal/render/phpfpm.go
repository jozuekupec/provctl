package render

import (
	"bytes"
	"fmt"
	"text/template"

	projecttemplates "provctl/templates"
)

// PHPFPMPool is the complete input for a generated per-subscription FPM pool.
type PHPFPMPool struct {
	Name        string
	Home        string
	Socket      string
	MaxChildren int
	MemoryLimit string
	UploadMax   string
	MaxExecTime int
	PhpErrorLog string
}

// RenderPHPFPMPool renders a pool with an isolated temporary and session path.
func RenderPHPFPMPool(pool PHPFPMPool) ([]byte, error) {
	if pool.Name == "" || pool.Home == "" || pool.Socket == "" || pool.MaxChildren <= 0 || pool.MemoryLimit == "" || pool.UploadMax == "" || pool.MaxExecTime <= 0 || pool.PhpErrorLog == "" {
		return nil, fmt.Errorf("incomplete PHP-FPM pool input")
	}
	templateContents, err := projecttemplates.Files.ReadFile("fpm/pool.conf.tmpl")
	if err != nil {
		return nil, fmt.Errorf("read PHP-FPM pool template: %w", err)
	}
	parsed, err := template.New("pool.conf.tmpl").Option("missingkey=error").Parse(string(templateContents))
	if err != nil {
		return nil, fmt.Errorf("parse PHP-FPM pool template: %w", err)
	}
	values := struct {
		PHPFPMPool
		StartServers int
		MinSpare     int
		MaxSpare     int
	}{PHPFPMPool: pool, StartServers: poolWorkers(pool.MaxChildren, 2), MinSpare: 1, MaxSpare: poolWorkers(pool.MaxChildren, 3)}
	var output bytes.Buffer
	if err := parsed.Execute(&output, values); err != nil {
		return nil, fmt.Errorf("render PHP-FPM pool template: %w", err)
	}
	return output.Bytes(), nil
}

func poolWorkers(maximum, preferred int) int {
	if maximum < preferred {
		return maximum
	}
	return preferred
}
