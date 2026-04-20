package gv8

import (
	_ "embed"
	"strings"
)

//go:embed internal/v8/VERSION
var bundledV8Version string

func Version() string {
	return strings.TrimSpace(bundledV8Version)
}
