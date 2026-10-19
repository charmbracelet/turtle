// Package basex implements the behavior shared by the base32 and base64
// utilities, on top of the RFC 4648 encodings of the standard library.
package basex

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
)

// ErrInvalidInput reports input that can't be decoded.
var ErrInvalidInput = errors.New("invalid input")

// padChar is the padding character of both codecs, like the standard library
// encodings use.
const padChar = '='

// defaultWrap is how many characters an encoded line holds, like GNU does.
const defaultWrap = 76

// chunkSize is how many bytes or characters are processed at a time. It's a
// multiple of every group size, so padding only ever shows up at the end of
// the encoded output.
const chunkSize = 1024 * 3 * 10

// Base32 is the codec used by the base32 utility.
var Base32 = Codec{
	Name:     "base32",
	enc:      base32.StdEncoding,
	dec:      base32.StdEncoding,
	alphabet: "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567",
	block:    5,
	group:    8,
}

// Base64 is the codec used by the base64 utility.
var Base64 = Codec{
	Name:     "base64",
	enc:      base64.StdEncoding,
	dec:      base64.StdEncoding,
	alphabet: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/",
	block:    3,
	group:    4,
}

// Codec encodes and decodes like the base32 and base64 utilities do.
type Codec struct {
	// Name is the program name, used on diagnostic messages.
	Name string

	enc, dec encoding
	alphabet string
	block    int
	group    int
}

// encoding is the API shared by the base32 and base64 encodings of the
// standard library.
type encoding interface {
	Encode(dst, src []byte)
	Decode(dst, src []byte) (int, error)
	EncodedLen(n int) int
	DecodedLen(n int) int
}

// Run encodes or decodes the input given by args and writes the result to the
// standard output. The standard input is used when no operand is given, and
// also when the operand is "-".
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (c *Codec) Run(ctx context.Context, rt *coreutils.Runtime, args []string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		c.errorf(rt, "%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 1 {
		c.errorf(rt, "extra operand '%s'", operands[1])
		return &coreutils.ExitError{Code: 2}
	}

	in := newInput(operands)
	if !in.stdin && in.name == "" {
		c.errorf(rt, "invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	r, err := c.open(rt, in)
	if err != nil {
		c.errorf(rt, "%s: %v", in.label(), unwrapPathError(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	defer r.Close()

	if opts.decode {
		err = c.Decode(ctx, rt.Stdout, r, opts.ignoreGarbage)
	} else {
		err = c.Encode(ctx, rt.Stdout, r, opts.wrap)
	}
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, ErrInvalidInput):
		c.errorf(rt, "invalid input")
		return &coreutils.ExitError{Code: 1}
	default:
		if writeErr, ok := errors.AsType[*writeError](err); ok {
			c.errorf(rt, "write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 1, Err: writeErr.err}
		}
		c.errorf(rt, "%s: %v", in.label(), unwrapPathError(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
}

// Encode writes the base-N encoding of r to w, breaking it into lines of wrap
// characters when wrap is greater than zero. Reading is interrupted when ctx
// is done.
func (c *Codec) Encode(ctx context.Context, w io.Writer, r io.Reader, wrap int) error {
	line := liner{w: w, wrap: wrap}
	src := make([]byte, chunkSize)
	dst := make([]byte, c.enc.EncodedLen(chunkSize))

	// carry holds the bytes of a group that isn't complete yet.
	carry := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := r.Read(src[carry:])
		total := carry + n
		full := total / c.block * c.block

		if full > 0 {
			enc := dst[:c.enc.EncodedLen(full)]
			c.enc.Encode(enc, src[:full])
			if werr := line.write(enc); werr != nil {
				return werr
			}
		}
		carry = copy(src, src[full:total])

		if err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
			if carry > 0 {
				enc := dst[:c.enc.EncodedLen(carry)]
				c.enc.Encode(enc, src[:carry])
				if werr := line.write(enc); werr != nil {
					return werr
				}
			}
			return line.finish()
		}
	}
}

// Decode writes the decoded bytes of r to w. <newline> characters are always
// ignored, and other characters outside of the alphabet are ignored when
// ignoreGarbage is true, making the input invalid otherwise. Reading is
// interrupted when ctx is done.
func (c *Codec) Decode(ctx context.Context, w io.Writer, r io.Reader, ignoreGarbage bool) error {
	src := make([]byte, chunkSize)
	dst := make([]byte, c.dec.DecodedLen(chunkSize+c.group))
	var pending []byte

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := r.Read(src)
		for _, b := range src[:n] {
			switch {
			case b == '\n':
			case b == padChar || strings.IndexByte(c.alphabet, b) >= 0:
				pending = append(pending, b)
			case ignoreGarbage:
			default:
				// GNU writes whatever was decoded before the garbage, so
				// the pending groups are flushed before failing.
				if _, derr := c.decodeGroups(w, pending, dst); derr != nil {
					return derr
				}
				return ErrInvalidInput
			}
		}

		var derr error
		if pending, derr = c.decodeGroups(w, pending, dst); derr != nil {
			return derr
		}
		if err == nil {
			continue
		}
		if !errors.Is(err, io.EOF) {
			return err
		}
		return c.decodeLast(w, pending, dst)
	}
}

// decodeGroups decodes every complete group of src and returns whatever is
// still pending. A group holding padding is decoded on its own, since the
// standard library only accepts it at the end of the input, while GNU decodes
// one group at a time and thus accepts it anywhere.
func (c *Codec) decodeGroups(w io.Writer, src, dst []byte) ([]byte, error) {
	for len(src) >= c.group {
		n := c.group
		if bytes.IndexByte(src[:c.group], padChar) < 0 {
			for n+c.group <= len(src) && bytes.IndexByte(src[n:n+c.group], padChar) < 0 {
				n += c.group
			}
		}
		if err := c.decode(w, src[:n], dst); err != nil {
			return src, err
		}
		src = src[n:]
	}
	return src, nil
}

// decodeLast decodes the pending input left at the end of the stream, padding
// it first when it comes without padding, like GNU does.
func (c *Codec) decodeLast(w io.Writer, pending, dst []byte) error {
	if len(pending) == 0 {
		return nil
	}
	if pending[len(pending)-1] != padChar {
		pending = append(pending, bytes.Repeat([]byte{padChar}, c.group-len(pending))...)
	}
	return c.decode(w, pending, dst)
}

// decode decodes a whole number of groups from src, writing them to w.
func (c *Codec) decode(w io.Writer, src, dst []byte) error {
	n, err := c.dec.Decode(dst[:c.dec.DecodedLen(len(src))], src)
	if err != nil {
		return ErrInvalidInput
	}
	if err := c.checkCanonical(dst[:n], src); err != nil {
		return err
	}
	if _, werr := w.Write(dst[:n]); werr != nil {
		return &writeError{err: werr}
	}
	return nil
}

// checkCanonical rejects an encoding that doesn't come back when the decoded
// bytes are encoded again, which is how GNU discards input with an unused
// padding length or with non-zero trailing bits.
func (c *Codec) checkCanonical(decoded, src []byte) error {
	if bytes.IndexByte(src, padChar) < 0 {
		return nil
	}
	enc := make([]byte, c.enc.EncodedLen(len(decoded)))
	c.enc.Encode(enc, decoded)
	if !bytes.Equal(enc, src) {
		return ErrInvalidInput
	}
	return nil
}

// errorf writes a diagnostic message to the standard error of rt.
func (c *Codec) errorf(rt *coreutils.Runtime, format string, args ...any) {
	fmt.Fprintf(rt.Stderr, c.Name+": "+format+"\n", args...)
}

// open returns a reader for the input.
func (c *Codec) open(rt *coreutils.Runtime, in input) (io.ReadCloser, error) {
	if in.stdin {
		if rt.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return io.NopCloser(rt.Stdin), nil
	}
	if rt.FS == nil {
		return nil, errors.New("no file system available")
	}
	return rt.FS.Open(filepath.ToSlash(rt.ResolvePath(in.name)))
}

// options holds what the -d, -i and -w options select.
type options struct {
	decode        bool
	ignoreGarbage bool
	wrap          int
}

// longOptions maps the long options of the utility to the short ones they
// stand for.
var longOptions = map[string]rune{
	"decode":         'd',
	"ignore-garbage": 'i',
	"wrap":           'w',
}

// parseFlags parses the options and operands. The -w option takes a number
// option-argument, either attached or in the next argument, and the last one
// given wins. Option parsing stops at the first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{wrap: defaultWrap}
	operands, err := getopt.Set{Valued: "w", Long: longOptions}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'd':
			opts.decode = true
		case 'i':
			opts.ignoreGarbage = true
		case 'w':
			wrap, err := parseWrap(value)
			if err != nil {
				return err
			}
			opts.wrap = wrap
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}

// parseWrap parses the number option-argument of -w: a non-negative amount of
// columns, where zero disables line wrapping. A value too big for an int
// disables it as well, like GNU does.
func parseWrap(value string) (int, error) {
	wrap, err := strconv.ParseInt(value, 10, 0)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) && wrap > 0 {
			return 0, nil
		}
		return 0, fmt.Errorf("invalid wrap size: '%s'", value)
	}
	if wrap < 0 {
		return 0, fmt.Errorf("invalid wrap size: '%s'", value)
	}
	return int(wrap), nil
}

// input describes the input operand: a pathname or the standard input.
type input struct {
	name  string
	stdin bool
}

// newInput maps the operands to an input, reading "-" as the standard input.
func newInput(operands []string) input {
	if len(operands) == 0 {
		return input{stdin: true}
	}
	return input{name: operands[0], stdin: operands[0] == "-"}
}

// label returns the name to use on diagnostic messages.
func (in input) label() string {
	if in.stdin && in.name == "" {
		return "standard input"
	}
	return in.name
}

// liner writes the encoded output, breaking it into lines of a given amount
// of characters.
type liner struct {
	w    io.Writer
	wrap int
	col  int
}

// write writes p, breaking the lines where the wrapping column asks for it.
func (l *liner) write(p []byte) error {
	if l.wrap == 0 {
		if _, err := l.w.Write(p); err != nil {
			return &writeError{err: err}
		}
		return nil
	}
	for len(p) > 0 {
		if l.col == l.wrap {
			if _, err := io.WriteString(l.w, "\n"); err != nil {
				return &writeError{err: err}
			}
			l.col = 0
		}
		n := min(l.wrap-l.col, len(p))
		if _, err := l.w.Write(p[:n]); err != nil {
			return &writeError{err: err}
		}
		l.col += n
		p = p[n:]
	}
	return nil
}

// finish terminates the last line, which only happens when wrapping is on.
func (l *liner) finish() error {
	if l.wrap == 0 || l.col == 0 {
		return nil
	}
	l.col = 0
	if _, err := io.WriteString(l.w, "\n"); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// writeError reports a failure writing to the standard output.
type writeError struct {
	err error
}

// Error implements the error interface.
func (e *writeError) Error() string {
	return e.err.Error()
}

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}
