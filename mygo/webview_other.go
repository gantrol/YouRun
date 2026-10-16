//go:build !windows

package main

func prepareWebView(_ string) (func(), error) { return func() {}, nil }
