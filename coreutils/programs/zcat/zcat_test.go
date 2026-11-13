package zcat

import (
	"errors"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/programs/gzip"
)

func TestUsageError(t *testing.T) {
	var stderr strings.Builder
	rt := &coreutils.Runtime{Stderr: &stderr}

	err := New(rt).Run(t.Context(), "-Z")

	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("Run() error = %v, want exit status 1", err)
	}
	want := "zcat: invalid option -- 'Z'\nTry 'zcat --help' for more information.\n"
	if got := stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var stdout strings.Builder
		rt := &coreutils.Runtime{Stdout: &stdout}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if got, want := stdout.String(), gzip.Usage("zcat"); got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		var stdout strings.Builder
		rt := &coreutils.Runtime{Stdout: &stdout}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		want := "zcat (charm.land/turtle/coreutils) " + coreutils.Version + "\nhttps://charm.land/turtle\n"
		if got := stdout.String(); got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})
}
