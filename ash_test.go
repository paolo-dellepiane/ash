package main

import (
	"os"
	"strings"
	"testing"
)

func TestLookForPath(t *testing.T) {
	res := lookForPath("__test__", "default")
	if !strings.Contains(res, "__test__") {
		t.Fatal("Path error", res)
	}
	f, err := os.ReadFile(res)
	if err != nil {
		t.Fatal(err)
	}
	if string(f) != "default" {
		t.Fatal("File content mismatch", string(f))
	}
	os.Remove(res)
}
