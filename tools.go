//go:build tools
// +build tools

package tools

import (
	// Document tool dependencies for version control
	_ "github.com/alecthomas/kong"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/goreleaser/goreleaser"
	_ "gotest.tools/gotestsum"
	_ "golang.org/x/vuln/cmd/govulncheck"
	_ "honnef.co/go/tools/cmd/staticcheck"
	_ "github.com/fzipp/gocyclo/cmd/gocyclo"
	_ "golang.org/x/tools/cmd/goimports"
	_ "github.com/vektra/mockery/v2"
)
