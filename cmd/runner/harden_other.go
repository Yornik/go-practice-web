//go:build !linux

package main

// harden does nothing outside Linux; the runner image is Linux only, and
// other systems serve local development.
func harden() error { return nil }
