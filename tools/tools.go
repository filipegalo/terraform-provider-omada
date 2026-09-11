//go:build tools

// Package tools pins the code-generation tools this repo uses, so `go mod
// tidy` keeps them in go.mod and every contributor and CI run generates docs
// with the same version.
package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
