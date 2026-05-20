package gv8

import (
	_ "embed"
	"strings"
)

//go:embed internal/v8/SOURCE_COMMIT
var bundledV8Commit string

func Version() string {
	return strings.TrimSpace(bundledV8Commit)
}
