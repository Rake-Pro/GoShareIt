//go:build darwin || windows

// Package main is the GoShareIt out-of-process annotation editor helper.
//
// The runnable entry point and IPC contract live in main.go. Every file in this
// package is build-tagged `darwin || windows` because the Gio UI it drives needs
// cgo on macOS and a GPU backend on both. On linux with CGO disabled the whole
// directory is excluded; `go build ./...` matches no buildable files here and
// cleanly skips it (verified), so no untagged stub is required - and an untagged
// `package main` without a main function would in fact break the linux build.
package main
