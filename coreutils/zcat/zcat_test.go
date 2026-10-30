package zcat

import (
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/gzip"
)

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
		want := "zcat (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})
}
