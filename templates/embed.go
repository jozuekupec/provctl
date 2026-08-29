// Package templates embeds the installed default configuration templates.
package templates

import "embed"

// Files contains the default templates distributed with provctl.
//
//go:embed apache/*.tmpl bootstrap/*.tmpl fpm/*.tmpl
var Files embed.FS
