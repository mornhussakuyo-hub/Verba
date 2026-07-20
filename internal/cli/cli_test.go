package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("")}
	if code := command.Run([]string{"version"}); code != 0 {
		t.Fatalf("exit code %d; stderr=%s", code, stderr.String())
	}
	if stdout.String() != "verba 0.1.0\n" {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestCheckReportsInvalidJSON(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "main.vrb")
	if err := os.WriteFile(path, []byte("module test\nembed json bad until done\n{bad}\ndone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("")}
	if code := command.Run([]string{"check", path}); code != 1 {
		t.Fatalf("exit code %d; stdout=%s; stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "JSON2001") {
		t.Fatalf("missing JSON diagnostic: %s", stderr.String())
	}
}
