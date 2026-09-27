package main

import "testing"

func TestListenAddressPreservesDefaultAndSupportsNativeLoopback(t *testing.T) {
	t.Parallel()

	if got := listenAddress("", "8080"); got != ":8080" {
		t.Fatalf("default listen address = %q, want %q", got, ":8080")
	}
	if got := listenAddress("127.0.0.1", "18087"); got != "127.0.0.1:18087" {
		t.Fatalf("native listen address = %q, want %q", got, "127.0.0.1:18087")
	}
}
