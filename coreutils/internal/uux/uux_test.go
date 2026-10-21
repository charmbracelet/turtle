package uux

import (
	"bytes"
	"io/fs"
	"testing"
)

func TestEncode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty input writes nothing", "", ""},
		{"three bytes become four characters", "cat", "#8V%T\n"},
		{"five bytes pad the last group", "hello", "%:&5L;&\\`\n"},
		{"a zero value is a backquote", "\x00\x01\x02\x03\x04\x05", "&``$\"`P0%\n"},
		{"a single byte", "x", "!>```\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := new(bytes.Buffer)
			err := Encode(t.Context(), out, bytes.NewReader([]byte(test.in)))
			if err != nil {
				t.Fatalf("Encode() error = %v, want nil", err)
			}
			if got := out.String(); got != test.want {
				t.Errorf("Encode() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEncodeSplitsLines(t *testing.T) {
	a45 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"45 bytes hold a single line of 60 characters",
			a45,
			"M86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A\n",
		},
		{
			"46 bytes spill onto a second line",
			a45 + "a",
			"M86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A86%A\n" +
				"!80``\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := new(bytes.Buffer)
			err := Encode(t.Context(), out, bytes.NewReader([]byte(test.in)))
			if err != nil {
				t.Fatalf("Encode() error = %v, want nil", err)
			}
			if got := out.String(); got != test.want {
				t.Errorf("Encode() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	data := make([]byte, LineBytes*3+5)
	for i := range data {
		data[i] = byte(i)
	}
	for _, size := range []int{0, 1, 2, 3, 4, 5, LineBytes - 1, LineBytes, LineBytes + 1, len(data)} {
		t.Run("", func(t *testing.T) {
			src := data[:size]

			out := new(bytes.Buffer)
			if err := Encode(t.Context(), out, bytes.NewReader(src)); err != nil {
				t.Fatalf("Encode() error = %v, want nil", err)
			}

			var got []byte
			for line := range bytes.SplitSeq(out.Bytes(), []byte("\n")) {
				if len(line) == 0 {
					continue
				}
				decoded, last, err := DecodeLine(line)
				if err != nil {
					t.Fatalf("DecodeLine(%q) error = %v, want nil", line, err)
				}
				if last {
					break
				}
				got = append(got, decoded...)
			}
			if !bytes.Equal(got, src) {
				t.Errorf("DecodeLine() = %x, want %x", got, src)
			}
		})
	}
}

func TestDecodeLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    []byte
		last    bool
		wantErr bool
	}{
		{"an empty line is invalid", "", nil, false, true},
		{"a backquote is the terminator", "`", nil, true, false},
		{"a space is the terminator", " ", nil, true, false},
		{"three bytes", "#8V%T", []byte("cat"), false, false},
		{"five bytes pad the last group", "%:&5L;&\\`", []byte("hello"), false, false},
		{
			"a space encodes a zero as well as a backquote",
			"%:&5L;&\\ ",
			[]byte("hello"),
			false,
			false,
		},
		{"a short line", "Mab", nil, false, true},
		{"a line shorter than its length says", "M", nil, false, true},
		{"a line announcing 46 bytes", "N", nil, false, true},
		{"a line announcing 63 bytes", "_", nil, false, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, last, err := DecodeLine([]byte(test.line))
			if test.wantErr {
				if err == nil {
					t.Errorf("DecodeLine(%q) error = nil, want an error", test.line)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeLine(%q) error = %v, want nil", test.line, err)
			}
			if !bytes.Equal(got, test.want) {
				t.Errorf("DecodeLine(%q) = %x, want %x", test.line, got, test.want)
			}
			if last != test.last {
				t.Errorf("DecodeLine(%q) last = %t, want %t", test.line, last, test.last)
			}
		})
	}
}

func TestBeginLine(t *testing.T) {
	tests := []struct {
		name   string
		base64 bool
		mode   fs.FileMode
		in     string
		want   string
	}{
		{"historical", false, 0o644, "out.txt", "begin 644 out.txt\n"},
		{"historical three digits", false, 0o700, "o", "begin 700 o\n"},
		{"base64", true, 0o644, "out.txt", "begin-base64 644 out.txt\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := BeginLine(test.base64, test.mode, test.in); got != test.want {
				t.Errorf("BeginLine() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseHeader(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Header
		ok   bool
	}{
		{"historical", "begin 644 out.txt", Header{Mode: "644", Name: "out.txt"}, true},
		{"base64", "begin-base64 600 a b", Header{Mode: "600", Name: "a b", Base64: true}, true},
		{"no name", "begin 644", Header{}, false},
		{"empty mode", "begin  out.txt", Header{}, false},
		{"double space", "begin  644 out.txt", Header{}, false},
		{"a data line", "M86%A86%A", Header{}, false},
		{"a base64 data line", "aGVsbG8=", Header{}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ParseHeader(test.in)
			if ok != test.ok {
				t.Errorf("ParseHeader(%q) ok = %t, want %t", test.in, ok, test.ok)
			}
			if got != test.want {
				t.Errorf("ParseHeader(%q) = %+v, want %+v", test.in, got, test.want)
			}
		})
	}
}
