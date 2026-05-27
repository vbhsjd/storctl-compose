package main

import (
	"testing"
	"time"
)

func TestParseTimeout(t *testing.T) {
	got, err := parseTimeout("5s")
	if err != nil {
		t.Fatal(err)
	}
	if got != 5*time.Second {
		t.Fatalf("timeout=%s", got)
	}
	if _, err := parseTimeout("0s"); err == nil {
		t.Fatal("zero timeout should fail")
	}
	if _, err := parseTimeout("bad"); err == nil {
		t.Fatal("invalid timeout should fail")
	}
}
