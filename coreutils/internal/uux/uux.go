// Package uux implements the formats shared by the uuencode and uudecode
// utilities: the historical encoding, with its length-prefixed lines, and the
// framing of the base64 alternative, both wrapped by a "begin" header.
package uux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

// LineBytes is how many input bytes each encoded line of the historical
// algorithm holds: 45 bytes, which become 60 characters.
const LineBytes = 45

// Base64Wrap is how many characters each line of the base64 encoding holds.
const Base64Wrap = 76

// Encode writes the historical encoding of r to w, one line per chunk of
// LineBytes bytes. Each line holds a length character (the amount of input
// bytes the line encodes, shifted by 0x20) followed by the encoded
// characters, each holding six bits of input also shifted by 0x20. A zero
// value is encoded as '`' rather than <space>, so no line ends on a blank.
//
// Reading is interrupted when ctx is done, and the context error is
// returned.
func Encode(ctx context.Context, w io.Writer, r io.Reader) error {
	src := make([]byte, LineBytes)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := io.ReadFull(r, src)
		if n > 0 {
			if werr := encodeLine(w, src[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
	}
}

// encodeLine writes a single line holding the historical encoding of data.
func encodeLine(w io.Writer, data []byte) error {
	line := make([]byte, 0, 2+(len(data)+2)/3*4)
	line = append(line, encode(len(data)))
	for i := 0; i < len(data); i += 3 {
		var b, c byte
		a := data[i]
		if i+1 < len(data) {
			b = data[i+1]
		}
		if i+2 < len(data) {
			c = data[i+2]
		}
		v := uint32(a)<<16 | uint32(b)<<8 | uint32(c)
		line = append(line,
			encode(int(v>>18)),
			encode(int(v>>12)),
			encode(int(v>>6)),
			encode(int(v)),
		)
	}
	line = append(line, '\n')
	_, err := w.Write(line)
	return err
}

// DecodeLine decodes a single line of the historical encoding, without its
// trailing newline. It returns the decoded bytes, and whether the line is
// the terminator: an empty line, or one whose length character decodes to
// zero, like "`" or " ". Both '`' and <space> are accepted as a zero value,
// as some encoders emit one and some the other.
func DecodeLine(line []byte) ([]byte, bool, error) {
	if len(line) == 0 {
		return nil, true, nil
	}
	n := int(decode(line[0]))
	if n == 0 {
		return nil, true, nil
	}
	chars := (n + 2) / 3 * 4
	if len(line) < 1+chars {
		return nil, false, errors.New("short line")
	}
	line = line[1:]
	out := make([]byte, 0, n)
	for i := 0; i < chars; i += 4 {
		v := uint32(decode(line[i]))<<18 |
			uint32(decode(line[i+1]))<<12 |
			uint32(decode(line[i+2]))<<6 |
			uint32(decode(line[i+3]))
		out = append(out, byte(v>>16), byte(v>>8), byte(v))
	}
	return out[:n], false, nil
}

// encode maps a six-bit value to its encoded character: the value shifted by
// 0x20, with a zero encoded as '`' rather than <space>.
func encode(v int) byte {
	c := byte((v & 0x3f) + 0x20)
	if c == ' ' {
		return '`'
	}
	return c
}

// decode maps an encoded character back to its six-bit value. Both '`' and
// <space> map to zero.
func decode(c byte) byte {
	return (c - 0x20) & 0x3f
}

// Header describes the "begin" line which opens an encoded input.
type Header struct {
	// Name is the pathname uudecode writes the decoded file to.
	Name string
	// Mode is the file access permission bits of the encoded file, in the
	// octal or symbolic notation of chmod.
	Mode string
	// Base64 reports the base64 encoding instead of the historical one.
	Base64 bool
}

// BeginLine returns the header line opening the encoding of a file with the
// given name and mode.
func BeginLine(base64 bool, mode fs.FileMode, name string) string {
	if base64 {
		return fmt.Sprintf("begin-base64 %03o %s\n", mode, name)
	}
	return fmt.Sprintf("begin %03o %s\n", mode, name)
}

// ParseHeader parses a line, without its trailing newline, as a "begin"
// line, reporting whether it's one. The rest of the line after the mode is
// taken as the name, which thus may hold blanks.
func ParseHeader(line string) (Header, bool) {
	if rest, ok := strings.CutPrefix(line, "begin-base64 "); ok {
		return cutHeader(rest, true)
	}
	if rest, ok := strings.CutPrefix(line, "begin "); ok {
		return cutHeader(rest, false)
	}
	return Header{}, false
}

// cutHeader splits the rest of a "begin" line into the mode and the name.
func cutHeader(rest string, base64 bool) (Header, bool) {
	mode, name, found := strings.Cut(rest, " ")
	if !found || mode == "" || name == "" {
		return Header{}, false
	}
	return Header{Name: name, Mode: mode, Base64: base64}, true
}
