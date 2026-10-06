package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportAndCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "openapi.yaml")
	if err := export(path, true); err == nil {
		t.Fatal("check should fail for a missing artifact")
	}
	if err := export(path, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# Code generated") || !strings.Contains(string(data), "openapi: 3.1.0") {
		t.Fatal("missing generated header or OpenAPI version")
	}
	if err := export(path, true); err != nil {
		t.Fatalf("generation is not deterministic: %v", err)
	}
	if err := os.WriteFile(path, []byte("outdated"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := export(path, true); err == nil {
		t.Fatal("check should fail for an outdated artifact")
	}
	data, _ = os.ReadFile(path)
	if string(data) != "outdated" {
		t.Fatal("check must not overwrite the artifact")
	}
}
