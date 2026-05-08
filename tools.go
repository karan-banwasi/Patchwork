//go:build tools

package main

// This file pins tool-only dependencies so `go mod tidy` does not remove them.
// By keeping the tool in go.mod, the //go:generate directive in main.go
// can use `go run` to robustly invoke the exact pinned version without
// requiring contributors or CI to manually `go install` the tool first.
//
// To regenerate the Windows manifest and icon:
//
//	go generate ./...
import _ "github.com/tc-hib/go-winres"
