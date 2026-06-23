package main

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	if got := version(); !strings.HasPrefix(got, "ghrun") {
		t.Fatalf("version() = %q, want prefix %q", got, "ghrun")
	}
}
