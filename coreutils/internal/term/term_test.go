package term

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTerminal(t *testing.T) {
	t.Run("a regular file", func(t *testing.T) {
		f, err := os.Create(filepath.Join(t.TempDir(), "f"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })

		if IsTerminal(int(f.Fd())) {
			t.Error("IsTerminal() = true for a regular file, want false")
		}
	})

	t.Run("a character device", func(t *testing.T) {
		f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })

		if IsTerminal(int(f.Fd())) {
			t.Error("IsTerminal() = true for a character device, want false")
		}
	})

	t.Run("a terminal", func(t *testing.T) {
		f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			t.Skipf("opening /dev/tty: %v", err)
		}
		t.Cleanup(func() { f.Close() })

		if !IsTerminal(int(f.Fd())) {
			t.Error("IsTerminal() = false for a terminal, want true")
		}
	})
}
