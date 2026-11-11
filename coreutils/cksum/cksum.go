// Package cksum implements the GNU cksum utility.
//
// It writes the checksum of each operand to the standard output, choosing
// the algorithm with the -a option: the legacy checksums of cksum and sum,
// and the digests of the standalone checksumming utilities. The standard
// input is used when no operand is given, and also when an operand is "-".
//
// The verification GNU selects with -c is not supported.
package cksum

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/digest"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// chunkSize is how many bytes of an input are read at a time, which is also
// how often the context is checked while reading.
const chunkSize = 32 * 1024

// Prog is the cksum core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the checksum of each operand, with the algorithm the -a option
// selects and in the output mode the other options choose. Long operations
// are interrupted when ctx is done, returning the context error as it is.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "cksum", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		stdopts.Errorf(p.Runtime, "cksum", "%v", err)
		stdopts.TryHelp(p.Runtime, "cksum")
		return &coreutils.ExitError{Code: 1}
	}
	a, err := opts.chosen()
	if err != nil {
		stdopts.Errorf(p.Runtime, "cksum", "%v", err)
		stdopts.TryHelp(p.Runtime, "cksum")
		return &coreutils.ExitError{Code: 1}
	}
	if err := opts.validate(a); err != nil {
		stdopts.Errorf(p.Runtime, "cksum", "%v", err)
		stdopts.TryHelp(p.Runtime, "cksum")
		return &coreutils.ExitError{Code: 1}
	}
	// The pathname is left out only when no file was given, so that an
	// explicit "-" names the standard input like the digest utilities do.
	unnamed := len(operands) == 0
	if unnamed {
		operands = []string{"-"}
	}
	var failed bool
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := p.checksum(ctx, a, opts, name, unnamed)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			stdopts.Errorf(p.Runtime, "cksum", "%s: %v", name, patherr.Unwrap(err))
			failed = true
			continue
		}
		if _, err := io.WriteString(p.Stdout, line); err != nil {
			stdopts.Errorf(p.Runtime, "cksum", "write error: %v", err)
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	if failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// options holds what the options of the utility select.
type options struct {
	algorithm string
	tag       bool
	untagged  bool
	base64    bool
	zero      bool

	length    string
	lengthSet bool
}

// longOptions maps the long options of the utility to the short ones they
// stand for. The ones without a short equivalent map to runes above the
// bytes a group of options can hold, keeping them out of the short syntax.
var longOptions = map[string]rune{
	"algorithm": 'a',
	"base64":    optBase64,
	"length":    'l',
	"tag":       optTag,
	"untagged":  optUntagged,
	"zero":      'z',
}

// The long options which have no short equivalent.
const (
	optBase64 = 0x100 + iota
	optTag
	optUntagged
)

// optSet describes the options of the utility.
var optSet = getopt.Set{Valued: "al", Long: longOptions}

// usage describes the options of the utility.
const usage = `Usage: cksum [OPTION]... [FILE]...
Print CRC checksums and byte counts, or the digests the -a option selects.

With no FILE, or when FILE is -, read standard input.

  -a, --algorithm=TYPE
         select the checksum type: crc (the default), sysv, bsd, crc32b, md5,
         sha1, sha2 or sha3
      --base64
         print base64-encoded digests, not hexadecimal
  -l, --length=BITS
         the digest length in bits, required with the sha2 and sha3 algorithms:
         224, 256, 384 or 512
      --tag
         create a tagged checksum (the default)
      --untagged
         create an untagged checksum, as the standalone digest utilities do
  -z, --zero
         end each output line with NUL, not newline
` + stdopts.Tail

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			opts.algorithm = value
		case 'l':
			opts.length, opts.lengthSet = value, true
		case 'z':
			opts.zero = true
		case optBase64:
			opts.base64 = true
		case optTag:
			opts.tag = true
		case optUntagged:
			opts.untagged = true
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

// chosen returns the algorithm the options select, reporting the values
// which name no algorithm and the lengths which fit none.
func (o options) chosen() (algorithm, error) {
	name := o.algorithm
	if name == "" {
		name = "crc"
	}
	switch name {
	case "sha2", "sha3":
		return o.resolved(name)
	}
	a, ok := algorithms[name]
	if !ok {
		return algorithm{}, fmt.Errorf("unrecognized algorithm '%s'", name)
	}
	if o.lengthSet {
		return algorithm{}, errors.New("the --length option is only supported with the sha2, sha3 and blake2b algorithms")
	}
	return a, nil
}

// resolved returns the sha2 or sha3 algorithm of the length --length gives,
// which those algorithms require.
func (o options) resolved(name string) (algorithm, error) {
	if !o.lengthSet {
		return algorithm{}, fmt.Errorf("the --length option is required with the %s algorithm", name)
	}
	bits, err := strconv.Atoi(o.length)
	if err != nil || bits != 224 && bits != 256 && bits != 384 && bits != 512 {
		return algorithm{}, fmt.Errorf("the --length option of the %s algorithm must be 224, 256, 384 or 512", name)
	}
	sha2 := map[int]func() hash.Hash{
		224: sha256.New224,
		256: sha256.New,
		384: sha512.New384,
		512: sha512.New,
	}
	sha3s := map[int]func() hash.Hash{
		224: func() hash.Hash { return sha3.New224() },
		256: func() hash.Hash { return sha3.New256() },
		384: func() hash.Hash { return sha3.New384() },
		512: func() hash.Hash { return sha3.New512() },
	}
	tag := "SHA" + o.length
	news := sha2
	if name == "sha3" {
		tag = "SHA3-" + o.length
		news = sha3s
	}
	return algorithm{name: name, digest: &digest.Algorithm{Tag: tag, New: news[bits]}}, nil
}

// validate reports the options which GNU refuses to take together.
func (o options) validate(a algorithm) error {
	switch {
	case o.tag && o.untagged:
		return errors.New("the --tag and --untagged options are mutually exclusive")
	case o.base64 && a.legacySum != nil:
		return fmt.Errorf("the --base64 option is not supported with the %s algorithm", a.name)
	}
	return nil
}

// algorithm is one of the checksums cksum computes.
type algorithm struct {
	// name is the value of the -a option which selects it.
	name string

	// digest is the hash of a digest algorithm, along with the tag its
	// tagged output shows. It is nil for the legacy checksums, whose
	// output has no tag.
	digest *digest.Algorithm

	// legacySum returns the checksum of r and the number printed as its
	// size, which is the byte count or the block count of sum.
	legacySum func(ctx context.Context, r io.Reader) (uint32, uint64, error)

	// padded tells whether the legacy output pads the checksum to five
	// digits and the size to five columns, as sum does.
	padded bool
}

// checksum returns the line the options write for the checksum of the file
// named name, opening and closing it.
func (p *Prog) checksum(ctx context.Context, a algorithm, o options, name string, unnamed bool) (string, error) {
	r, err := input.Open(p.Runtime, ctx, input.New(name))
	if err != nil {
		return "", err
	}
	line, err := a.line(ctx, r, o, name, unnamed)
	if closeErr := r.Close(); err == nil {
		err = closeErr
	}
	return line, err
}

// line returns the line the options write for the checksum of r, ending it
// with the byte --zero asks for.
func (a algorithm) line(ctx context.Context, r io.Reader, o options, name string, unnamed bool) (string, error) {
	var s string
	if a.legacySum != nil {
		sum, size, err := a.legacySum(ctx, r)
		if err != nil {
			return "", err
		}
		s = a.legacyLine(sum, size, name, unnamed)
	} else {
		d, _, err := hashStream(ctx, r, a.digest.New())
		if err != nil {
			return "", err
		}
		s = a.digestLine(o, name, d)
	}
	if o.zero {
		return s + "\x00", nil
	}
	return s + "\n", nil
}

// legacyLine returns the line written for the checksum of a file, which
// names it only when an operand was given.
func (a algorithm) legacyLine(sum uint32, size uint64, name string, unnamed bool) string {
	format := "%d %d"
	if a.padded {
		format = "%05d %5d"
	}
	line := fmt.Sprintf(format, sum, size)
	if !unnamed {
		line += " " + name
	}
	return line
}

// digestLine returns the line written for the digest d of a file name, in
// the tagged output the digest algorithms default to, or the untagged one
// of the standalone utilities. The input mode marker is always the one of
// text, which reading keeps regardless of the system.
func (a algorithm) digestLine(o options, name string, d []byte) string {
	encoded := hex.EncodeToString(d)
	if o.base64 {
		encoded = base64.StdEncoding.EncodeToString(d)
	}
	// Without --zero, a file name holding a backslash or a line break is
	// escaped, which needs the line to start with a backslash.
	escaped := !o.zero && problematic(name)
	if escaped {
		name = nameEscaper.Replace(name)
	}
	var b strings.Builder
	if escaped {
		b.WriteByte('\\')
	}
	if o.untagged {
		b.WriteString(encoded)
		b.WriteString("  ")
		b.WriteString(name)
	} else {
		b.WriteString(a.digest.Tag)
		b.WriteString(" (")
		b.WriteString(name)
		b.WriteString(") = ")
		b.WriteString(encoded)
	}
	return b.String()
}

// hashStream returns the digest of r, reading it in chunks so that the
// context is checked while it runs.
func hashStream(ctx context.Context, r io.Reader, h hash.Hash) ([]byte, int64, error) {
	var size int64
	buf := make([]byte, chunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		n, readErr := r.Read(buf)
		h.Write(buf[:n])
		size += int64(n)
		if readErr != nil {
			if readErr != io.EOF {
				return nil, 0, readErr
			}
			return h.Sum(nil), size, nil
		}
	}
}

// stream feeds every chunk of r to write, returning the number of bytes it
// read, checking the context as it goes.
func stream(ctx context.Context, r io.Reader, write func([]byte)) (int64, error) {
	var size int64
	buf := make([]byte, chunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		n, readErr := r.Read(buf)
		write(buf[:n])
		size += int64(n)
		if readErr != nil {
			if readErr != io.EOF {
				return 0, readErr
			}
			return size, nil
		}
	}
}

// algorithms maps the values of the -a option to the algorithms they
// select. The sha2 and sha3 entries are resolved from --length instead.
var algorithms = map[string]algorithm{
	"sysv":   {name: "sysv", legacySum: sysvSum, padded: true},
	"bsd":    {name: "bsd", legacySum: bsdSum, padded: true},
	"crc":    {name: "crc", legacySum: crcSum},
	"crc32b": {name: "crc32b", legacySum: crc32bSum},
	"md5":    {name: "md5", digest: &digest.Algorithm{Tag: "MD5", New: md5.New}},
	"sha1":   {name: "sha1", digest: &digest.Algorithm{Tag: "SHA1", New: sha1.New}},
	"sha224": {name: "sha224", digest: &digest.Algorithm{Tag: "SHA224", New: sha256.New224}},
	"sha256": {name: "sha256", digest: &digest.Algorithm{Tag: "SHA256", New: sha256.New}},
	"sha384": {name: "sha384", digest: &digest.Algorithm{Tag: "SHA384", New: sha512.New384}},
	"sha512": {name: "sha512", digest: &digest.Algorithm{Tag: "SHA512", New: sha512.New}},
}

// crcSum returns the checksum of r and its byte count, as the algorithm of
// the cksum specification computes them.
func crcSum(ctx context.Context, r io.Reader) (uint32, uint64, error) {
	var c crc
	size, err := stream(ctx, r, c.write)
	if err != nil {
		return 0, 0, err
	}
	c.writeSize(size)
	return ^c.value, uint64(size), nil
}

// crc32bSum returns the CRC-32 of r and its byte count, as the polynomial
// of the IEEE 802.3 standard computes it.
func crc32bSum(ctx context.Context, r io.Reader) (uint32, uint64, error) {
	c := crc32.NewIEEE()
	size, err := stream(ctx, r, func(b []byte) { c.Write(b) })
	if err != nil {
		return 0, 0, err
	}
	return c.Sum32(), uint64(size), nil
}

// sysvSum returns the checksum of r and its block count, as the System V
// sum computes them.
func sysvSum(ctx context.Context, r io.Reader) (uint32, uint64, error) {
	var sum uint32
	size, err := stream(ctx, r, func(b []byte) {
		for _, x := range b {
			sum += uint32(x)
		}
	})
	if err != nil {
		return 0, 0, err
	}
	sum = (sum & 0xffff) + (sum >> 16)
	sum = (sum & 0xffff) + (sum >> 16)
	return sum, uint64((size + 511) / 512), nil
}

// bsdSum returns the checksum of r and its block count, as the BSD sum
// computes them.
func bsdSum(ctx context.Context, r io.Reader) (uint32, uint64, error) {
	var sum uint32
	size, err := stream(ctx, r, func(b []byte) {
		for _, x := range b {
			sum = (sum>>1 | (sum&1)<<15) + uint32(x)
			sum &= 0xffff
		}
	})
	if err != nil {
		return 0, 0, err
	}
	return sum, uint64((size + 1023) / 1024), nil
}

// crc holds the checksum of a stream, as the algorithm of the cksum
// specification computes it.
type crc struct {
	value uint32
}

// write updates the checksum with the given bytes.
func (c *crc) write(data []byte) {
	for _, b := range data {
		c.value = c.value<<8 ^ crcTable[byte(c.value>>24)^b]
	}
}

// writeSize folds the size of the input into the checksum, as the length
// octets of the algorithm, least significant first.
func (c *crc) writeSize(size int64) {
	for size > 0 {
		c.value = c.value<<8 ^ crcTable[byte(c.value>>24)^byte(size)]
		size >>= 8
	}
}

// crcTable is the lookup table of the polynomial of the ISO/IEC 8802-3
// standard, as the cksum specification defines it.
var crcTable = func() [256]uint32 {
	var table [256]uint32
	for i := range table {
		c := uint32(i) << 24
		for range 8 {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04c11db7
			} else {
				c <<= 1
			}
		}
		table[i] = c
	}
	return table
}()

// nameEscaper translates the characters which are escaped on output.
var nameEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`)

// problematic reports whether a file name holds characters which have to be
// escaped for the lines it appears in to be parsed back.
func problematic(s string) bool {
	return strings.ContainsAny(s, "\\\n\r")
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
