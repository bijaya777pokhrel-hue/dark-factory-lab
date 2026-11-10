package main_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const targetSentence = "The meal was perhaps good, but dessert was horrible."

func TestMainBinary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bin := filepath.Join(dir, "semanticfont")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	textFile := filepath.Join(dir, "sentence.txt")
	if err := os.WriteFile(textFile, []byte(targetSentence), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		args      []string
		stdin     string
		wantCode  int
		wantOut   []string
		emptyOut  bool
		wantStErr bool
	}{
		{name: "target sentence on stdin", stdin: targetSentence, wantOut: []string{"<!doctype html>", ">horrible</span>."}},
		{name: "file argument", args: []string{textFile}, wantOut: []string{"<!doctype html>", ">perhaps</span>"}},
		{name: "help", args: []string{"--help"}, wantOut: []string{"Usage"}},
		{name: "bad flag", args: []string{"-x"}, wantCode: 2, emptyOut: true, wantStErr: true},
		{name: "missing file", args: []string{filepath.Join(dir, "missing.txt")}, wantCode: 1, emptyOut: true, wantStErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(bin, tc.args...)
			cmd.Stdin = strings.NewReader(tc.stdin)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			code := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code = exitErr.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if code != tc.wantCode {
				t.Fatalf("exit code %d, want %d (stderr %q)", code, tc.wantCode, stderr.String())
			}
			if tc.emptyOut && stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if tc.wantStErr != (stderr.Len() != 0) {
				t.Errorf("stderr = %q, want non-empty: %v", stderr.String(), tc.wantStErr)
			}
			for _, s := range tc.wantOut {
				if !strings.Contains(stdout.String(), s) {
					t.Errorf("stdout lacks %q", s)
				}
			}
		})
	}
}
