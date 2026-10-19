package zcore

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"strings"
	"testing"
)

func TestSniff(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want Algorithm
	}{
		{"gzip", []byte{0x1f, 0x8b}, Gzip},
		{"compress", []byte{0x1f, 0x9d}, LZW},
		{"plain", []byte("plain"), none},
		{"empty", nil, none},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sniff(bufio.NewReader(bytes.NewReader(test.data))); got != test.want {
				t.Errorf("sniff() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSourceDecode(t *testing.T) {
	t.Run("restores a gzip member", func(t *testing.T) {
		var data bytes.Buffer
		writer := gzip.NewWriter(&data)
		if _, err := io.WriteString(writer, "alpha\n"); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		var output strings.Builder
		n, err := openSource(bytes.NewReader(data.Bytes())).decode(t.Context(), &output, false)
		if err != nil {
			t.Fatalf("decode() error = %v", err)
		}
		if output.String() != "alpha\n" {
			t.Errorf("output = %q, want alpha\\n", output.String())
		}
		if n != int64(len("alpha\n")) {
			t.Errorf("bytes = %d, want %d", n, len("alpha\n"))
		}
	})

	t.Run("refuses plain data without passthrough", func(t *testing.T) {
		if _, err := openSource(strings.NewReader("plain")).decode(t.Context(), io.Discard, false); err != errNotCompressed {
			t.Errorf("decode() error = %v, want %v", err, errNotCompressed)
		}
	})

	t.Run("passes plain data through when allowed", func(t *testing.T) {
		var output strings.Builder
		if _, err := openSource(strings.NewReader("plain")).decode(t.Context(), &output, true); err != nil {
			t.Fatalf("decode() error = %v", err)
		}
		if output.String() != "plain" {
			t.Errorf("output = %q, want plain", output.String())
		}
	})

	t.Run("recognizes .Z but doesn't decode it yet", func(t *testing.T) {
		if _, err := openSource(bytes.NewReader([]byte{0x1f, 0x9d})).decode(t.Context(), io.Discard, false); err != errUnsupported {
			t.Errorf("decode() error = %v, want %v", err, errUnsupported)
		}
	})

	t.Run("reads a cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := openSource(strings.NewReader("plain")).decode(ctx, io.Discard, true); err != context.Canceled {
			t.Errorf("decode() error = %v, want context.Canceled", err)
		}
	})
}

func TestSuffix(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"default gzip", Options{Algorithm: Gzip}, ".gz"},
		{"default LZW", Options{Algorithm: LZW}, ".Z"},
		{"override", Options{Algorithm: Gzip, Suffix: ".zip"}, ".zip"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.opts.suffix(); got != test.want {
				t.Errorf("suffix() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStripSuffix(t *testing.T) {
	tests := []struct {
		name  string
		opts  Options
		input string
		want  string
		ok    bool
	}{
		{"gzip", Options{Algorithm: Gzip}, "file.txt.gz", "file.txt", true},
		{"short gzip", Options{Algorithm: Gzip}, "file.txt.z", "file.txt", true},
		{"compress", Options{Algorithm: Gzip}, "file.txt.Z", "file.txt", true},
		{"tar gzip", Options{Algorithm: Gzip}, "archive.tgz", "archive.tar", true},
		{"tar deflate", Options{Algorithm: Gzip}, "archive.tz", "archive.tar", true},
		{"custom suffix", Options{Algorithm: Gzip, Suffix: ".zip"}, "file.zip", "file", true},
		{"plain", Options{Algorithm: Gzip}, "file.txt", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := stripSuffix(test.input, test.opts)
			if got != test.want || ok != test.ok {
				t.Errorf("stripSuffix() = %q, %v, want %q, %v", got, ok, test.want, test.ok)
			}
		})
	}
}

func TestRatio(t *testing.T) {
	tests := []struct {
		compressed int64
		original   int64
		want       string
	}{
		{50, 100, "50.00%"},
		{150, 100, "-50.00%"},
		{1, 0, "0.00%"},
	}

	for _, test := range tests {
		if got := ratio(test.compressed, test.original); got != test.want {
			t.Errorf("ratio(%d, %d) = %q, want %q", test.compressed, test.original, got, test.want)
		}
	}
}

func TestCheckName(t *testing.T) {
	if err := checkName("dir/" + strings.Repeat("x", nameMax)); err != nil {
		t.Errorf("checkName() error = %v, want nil", err)
	}
	if err := checkName("dir/" + strings.Repeat("x", nameMax+1)); err == nil {
		t.Error("checkName() error = nil, want error")
	}
}
