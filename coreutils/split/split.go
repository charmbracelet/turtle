// Package split implements the GNU split utility.
//
// It writes pieces of the input file to files named after a prefix and a
// suffix ("xaa", "xab", ... by default; digits or hexadecimal digits with -d
// or -x), either by line count, by byte count, by complete lines per size,
// or by a number of chunks. The --filter and --separator options of the GNU
// manual are not supported, and the suffix length doesn't grow automatically
// when the suffixes run out.
package split

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/sizeparse"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// Prog is the split core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run reads the input file and writes its pieces, returning an exit status
// of 2 for invalid usage and 1 for any processing error. The k/n forms of
// -n write their chunk to the standard output instead of to a file.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "split", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "split")
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 2 {
		p.errorf("extra operand '%s'", operands[2])
		stdopts.TryHelp(p.Runtime, "split")
		return &coreutils.ExitError{Code: 2}
	}
	name := "-"
	prefix := "x"
	if len(operands) >= 1 {
		name = operands[0]
	}
	if len(operands) == 2 {
		prefix = operands[1]
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	w := &pieceWriter{
		rt: p.Runtime, prefix: prefix, suffixLen: opts.suffixLen,
		numeric: opts.numeric, hex: opts.hex, from: opts.from, count: opts.from,
		addSuffix: opts.addSuffix,
	}
	if len(baseName(prefix))+opts.suffixLen+len(opts.addSuffix) > nameMax {
		p.errorf("%s: file name too long", w.nextName())
		return &coreutils.ExitError{Code: 1}
	}

	r, err := input.Open(p.Runtime, ctx, input.New(name))
	if err != nil {
		p.errorf("%s: %v", name, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}
	defer r.Close()

	switch {
	case opts.chunks != nil:
		err = p.splitChunks(ctx, r, w, opts)
	case opts.lineBytes >= 0:
		err = splitLineBytes(ctx, r, w, opts.lineBytes)
	case opts.bytes >= 0:
		err = splitBytes(ctx, r, w, opts.bytes)
	default:
		err = splitLines(ctx, r, w, opts.lines)
	}
	if err != nil {
		var writeErr *writeError
		var readErr *readError
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errSuffixesExhausted):
			p.errorf("%v", err)
		case errors.As(err, &writeErr):
			p.errorf("%s: %v", writeErr.name, patherr.Unwrap(writeErr.err))
		case errors.As(err, &readErr):
			p.errorf("%s: %v", name, patherr.Unwrap(readErr.err))
		default:
			p.errorf("%v", err)
		}
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// nameMax is the assumed NAME_MAX limit of the output directory.
const nameMax = 255

// pieceWriter creates the output files and keeps track of the suffixes.
type pieceWriter struct {
	rt        *coreutils.Runtime
	prefix    string
	suffixLen int
	numeric   bool // -d: decimal digits
	hex       bool // -x: hexadecimal digits
	from      int64
	addSuffix string
	count     int64 // the suffix number of the current piece
	file      fsx.File
	name      string
}

// errSuffixesExhausted signals that no more suffixes can be formed.
var errSuffixesExhausted = errors.New("output file suffixes exhausted")

// base returns the size of the suffix alphabet.
func (w *pieceWriter) base() int64 {
	switch {
	case w.numeric:
		return 10
	case w.hex:
		return 16
	default:
		return 26
	}
}

// digit renders one digit of the suffix alphabet.
func (w *pieceWriter) digit(v int64) byte {
	switch {
	case w.numeric:
		return byte('0' + v)
	case w.hex:
		return "0123456789abcdef"[v]
	default:
		return byte('a' + v)
	}
}

// nextName returns the output name for the current count.
func (w *pieceWriter) nextName() string {
	suffix := make([]byte, w.suffixLen)
	n := w.count
	for i := w.suffixLen - 1; i >= 0; i-- {
		suffix[i] = w.digit(n % w.base())
		n /= w.base()
	}
	return w.prefix + string(suffix) + w.addSuffix
}

// open creates the file for the current count, closing the previous one if
// any. The count advances, so the next piece is named after the next
// suffix.
func (w *pieceWriter) open() error {
	if err := w.close(); err != nil {
		return err
	}
	if w.count-w.from >= intPow(int(w.base()), w.suffixLen) {
		return errSuffixesExhausted
	}
	name := w.nextName()
	f, err := w.rt.FS.OpenFile(filepath.ToSlash(w.rt.ResolvePath(name)), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^w.rt.Umask.Perm())
	if err != nil {
		return &writeError{name: name, err: err}
	}
	w.file = f
	w.name = name
	w.count++
	return nil
}

// begin switches to the piece of the given number, which is relative to the
// first suffix. The piece's file is created lazily, by the first write, or
// by create when it must exist even empty.
func (w *pieceWriter) begin(piece int64) error {
	if err := w.close(); err != nil {
		return err
	}
	w.count = w.from + piece
	return nil
}

// create opens the file of the current piece without writing to it.
func (w *pieceWriter) create() error {
	return w.open()
}

// write appends data to the current piece, opening one first if needed.
func (w *pieceWriter) write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if w.file == nil {
		if err := w.open(); err != nil {
			return err
		}
	}
	if _, err := w.file.Write(data); err != nil {
		return &writeError{name: w.name, err: err}
	}
	return nil
}

// close closes the current piece, if any, so the next write starts a new
// one.
func (w *pieceWriter) close() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return &writeError{name: w.name, err: err}
	}
	return nil
}

// splitLines writes pieces of at most n lines each.
func splitLines(ctx context.Context, r io.Reader, w *pieceWriter, n int64) error {
	br := bufio.NewReader(r)
	var count int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if count == n {
				if err := w.close(); err != nil {
					return err
				}
				count = 0
			}
			if err := w.write(line); err != nil {
				return err
			}
			count++
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return &readError{err: err}
		}
	}
	return w.close()
}

// splitBytes writes pieces of at most n bytes each.
func splitBytes(ctx context.Context, r io.Reader, w *pieceWriter, n int64) error {
	buf := make([]byte, 32*1024)
	var remaining int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		nr, err := r.Read(buf)
		data := buf[:nr]
		for len(data) > 0 {
			if remaining == 0 {
				if err := w.close(); err != nil {
					return err
				}
				remaining = n
			}
			chunk := min(int64(len(data)), remaining)
			if err := w.write(data[:chunk]); err != nil {
				return err
			}
			data = data[chunk:]
			remaining -= chunk
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return &readError{err: err}
		}
	}
	return w.close()
}

// splitLineBytes writes pieces holding as many complete lines as fit in n
// bytes. A line longer than n bytes is broken into multiple pieces.
func splitLineBytes(ctx context.Context, r io.Reader, w *pieceWriter, n int64) error {
	br := bufio.NewReader(r)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if size > 0 && size+int64(len(line)) > n {
				if err := w.close(); err != nil {
					return err
				}
				size = 0
			}
			for off := 0; off < len(line); {
				chunk := line[off:]
				if size+int64(len(chunk)) > n {
					chunk = chunk[:max(n-size, 0)]
				}
				if len(chunk) == 0 {
					if err := w.close(); err != nil {
						return err
					}
					size = 0
					continue
				}
				if err := w.write(chunk); err != nil {
					return err
				}
				size += int64(len(chunk))
				off += len(chunk)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return &readError{err: err}
		}
	}
	return w.close()
}

// chunkSpec is the parsed -n operand: n chunks, optionally emitting only
// the k'th one, without splitting lines (l) or in a round-robin
// distribution (r).
type chunkSpec struct {
	mode byte // 'b' for bytes, 'l' for lines, 'r' for round-robin
	n    int64
	k    int64 // 1-based, 0 for all chunks
}

// splitChunks splits the whole input into the -n chunks, writing the pieces
// to files, or the selected chunk to the standard output when k is given.
func (p *Prog) splitChunks(ctx context.Context, r io.Reader, w *pieceWriter, opts options) error {
	data, lines, starts, err := readAll(ctx, r)
	if err != nil {
		return err
	}
	spec := *opts.chunks
	switch spec.mode {
	case 'r':
		return p.roundRobin(ctx, w, lines, spec, opts.elide)
	case 'l':
		return p.lineChunks(ctx, w, data, lines, starts, spec, opts.elide)
	default:
		return p.byteChunks(ctx, w, data, spec, opts.elide)
	}
}

// byteChunks splits the input into n byte ranges, the early ones one byte
// longer when the size isn't a multiple of n.
func (p *Prog) byteChunks(ctx context.Context, w *pieceWriter, data []byte, spec chunkSpec, elide bool) error {
	total := int64(len(data))
	base, rem := total/spec.n, total%spec.n
	chunk := func(c int64) (int64, int64) {
		start := c*base + min(c, rem)
		size := base
		if c < rem {
			size++
		}
		return start, size
	}
	if spec.k > 0 {
		start, size := chunk(spec.k - 1)
		return p.writeStdout(ctx, data[start:start+size])
	}
	for i := int64(0); i < spec.n; i++ {
		if err := w.begin(i); err != nil {
			return err
		}
		start, size := chunk(i)
		if size == 0 {
			if !elide {
				if err := w.create(); err != nil {
					return err
				}
			}
			continue
		}
		if err := w.write(data[start : start+size]); err != nil {
			return err
		}
	}
	return w.close()
}

// lineChunks splits the input into n regions of approximately equal size,
// each holding the lines which start inside it.
func (p *Prog) lineChunks(ctx context.Context, w *pieceWriter, data []byte, lines [][]byte, starts []int, spec chunkSpec, elide bool) error {
	// startLines[i] is the first line of region i: the first line which
	// starts at or after the i'th size boundary.
	startLines := make([]int, spec.n+1)
	for i := range startLines {
		bound := int64(len(data)) * int64(i) / spec.n
		si := sortSearch(starts, bound)
		// A boundary inside a line: the line belongs to the region the
		// boundary opens.
		if si < len(starts) && int64(starts[si]) < bound {
			si++
		}
		startLines[i] = si
	}
	write := func(c, from, to int64) error {
		if spec.k > 0 {
			for i := from; i < to; i++ {
				if err := p.writeStdout(ctx, lines[i]); err != nil {
					return err
				}
			}
			return nil
		}
		if err := w.begin(c); err != nil {
			return err
		}
		if from == to {
			if !elide {
				if err := w.create(); err != nil {
					return err
				}
			}
			return w.close()
		}
		for i := from; i < to; i++ {
			if err := w.write(lines[i]); err != nil {
				return err
			}
		}
		return w.close()
	}
	if spec.k > 0 {
		return write(spec.k, int64(startLines[spec.k-1]), int64(startLines[spec.k]))
	}
	for i := int64(0); i < spec.n; i++ {
		if err := write(i, int64(startLines[i]), int64(startLines[i+1])); err != nil {
			return err
		}
	}
	return nil
}

// roundRobin distributes the lines to the chunks round-robin: line i goes
// to chunk i mod n.
func (p *Prog) roundRobin(ctx context.Context, w *pieceWriter, lines [][]byte, spec chunkSpec, elide bool) error {
	write := func(c int64) error {
		if spec.k > 0 {
			for i, line := range lines {
				if int64(i)%spec.n == spec.k-1 {
					if err := p.writeStdout(ctx, line); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if err := w.begin(c); err != nil {
			return err
		}
		empty := true
		for i, line := range lines {
			if int64(i)%spec.n == c {
				empty = false
				if err := w.write(line); err != nil {
					return err
				}
			}
		}
		if empty && !elide {
			if err := w.create(); err != nil {
				return err
			}
		}
		return w.close()
	}
	if spec.k > 0 {
		return write(spec.k)
	}
	for c := int64(0); c < spec.n; c++ {
		if err := write(c); err != nil {
			return err
		}
	}
	return nil
}

// writeStdout writes data to the standard output, stopping when ctx is
// done.
func (p *Prog) writeStdout(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := p.Stdout.Write(data); err != nil {
		return &writeError{name: "-", err: err}
	}
	return nil
}

// readAll reads the whole input, returning its bytes, its lines with their
// terminators, and the byte offset at which each line starts.
func readAll(ctx context.Context, r io.Reader) (data []byte, lines [][]byte, starts []int, err error) {
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		nr, rerr := r.Read(buf)
		data = append(data, buf[:nr]...)
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return nil, nil, nil, &readError{err: rerr}
		}
	}
	if len(data) > 0 {
		starts = append(starts, 0)
		for i, b := range data {
			if b == '\n' && i+1 < len(data) {
				starts = append(starts, i+1)
			}
		}
		lines = make([][]byte, len(starts))
		for i, start := range starts {
			end := len(data)
			if i+1 < len(starts) {
				end = starts[i+1]
			}
			lines[i] = data[start:end]
		}
	}
	return data, lines, starts, nil
}

// sortSearch returns the smallest index at which v could be inserted into
// the sorted slice without disordering it.
func sortSearch(sorted []int, v int64) int {
	i, j := 0, len(sorted)
	for i < j {
		h := int(uint(i+j) >> 1)
		if int64(sorted[h]) < v {
			i = h + 1
		} else {
			j = h
		}
	}
	return i
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "split: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	lines     int64
	bytes     int64
	lineBytes int64
	chunks    *chunkSpec
	suffixLen int
	numeric   bool
	hex       bool
	from      int64
	addSuffix string
	elide     bool
}

// longOptions maps the long options of split to the short ones they stand
// for.
var longOptions = map[string]rune{
	"bytes":             'b',
	"lines":             'l',
	"line-bytes":        'C',
	"number":            'n',
	"suffix-length":     'a',
	"numeric-suffixes":  'd',
	"hex-suffixes":      'x',
	"elide-empty-files": 'e',
	"additional-suffix": optAddSuffix,
}

// optAddSuffix is a rune which no short option can reach, so
// --additional-suffix stays long-only.
const optAddSuffix rune = 0x100

// optSet describes the options of split.
var optSet = getopt.Set{
	Valued:   "ablnC" + string(optAddSuffix),
	Optional: "dx",
	Long:     longOptions,
}

// usage describes the options of split.
const usage = `Usage: split [OPTION]... [FILE [PREFIX]]
Output pieces of FILE to PREFIXaa, PREFIXab, ...;
default FILE is stdin and default PREFIX is 'x'.

  -a, --suffix-length=N
         generate suffixes of length N (default 2)
  -b, --bytes=SIZE
         put SIZE bytes per output file
  -C, --line-bytes=SIZE
         put at most SIZE bytes of lines per output file
  -d, --numeric-suffixes[=FROM]
         use numeric suffixes instead of alphabetic; FROM changes the
         start value (default 0)
  -e, --elide-empty-files
         do not generate empty output files
  -l, --lines=NUMBER
         put NUMBER lines per output file
  -n, --number=CHUNKS
         generate CHUNKS output files
  -x, --hex-suffixes[=FROM]
         like -d, but use hexadecimal suffixes
      --additional-suffix=SUFFIX
         append SUFFIX to file names
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--". The -d and -x options take an optional attached
// starting number.
func parseFlags(args []string) (options, []string, error) {
	opts := options{lines: 1000, bytes: -1, lineBytes: -1, suffixLen: 2}
	var mode byte
	operands, err := optSet.Parse(expandSuffixOpts(args), func(opt rune, value string) error {
		switch opt {
		case 'a':
			n, err := strconv.ParseInt(value, 10, 0)
			if err != nil || n < 0 || !isDecimal(value) {
				return fmt.Errorf("invalid suffix length: '%s'", value)
			}
			// A length of zero selects the default length, as if -a was
			// not given.
			opts.suffixLen = 2
			if n > 0 {
				opts.suffixLen = int(n)
			}
		case 'b':
			if mode != 0 {
				return errors.New("cannot split in more than one way")
			}
			mode = 'b'
			n, err := sizeparse.Parse(value)
			if err != nil || n < 1 {
				return fmt.Errorf("invalid number of bytes: '%s'", value)
			}
			opts.bytes = n
		case 'l':
			if mode != 0 {
				return errors.New("cannot split in more than one way")
			}
			mode = 'l'
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 1 || !isDecimal(value) {
				return fmt.Errorf("invalid number of lines: '%s'", value)
			}
			opts.lines = n
		case 'C':
			if mode != 0 {
				return errors.New("cannot split in more than one way")
			}
			mode = 'C'
			n, err := sizeparse.Parse(value)
			if err != nil || n < 1 {
				return fmt.Errorf("invalid number of bytes: '%s'", value)
			}
			opts.lineBytes = n
		case 'n':
			if mode != 0 {
				return errors.New("cannot split in more than one way")
			}
			mode = 'n'
			spec, err := parseChunks(value)
			if err != nil {
				return err
			}
			opts.chunks = &spec
		case 'd', 'x':
			if opt == 'd' && opts.hex || opt == 'x' && opts.numeric {
				return errors.New("cannot combine -d and -x")
			}
			if value != "" {
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || n < 0 || !isDecimal(value) {
					return fmt.Errorf("invalid suffix number: '%s'", value)
				}
				opts.from = n
			}
			if opt == 'd' {
				opts.numeric = true
			} else {
				opts.hex = true
			}
		case 'e':
			opts.elide = true
		case optAddSuffix:
			if strings.Contains(value, "/") {
				return fmt.Errorf("additional suffix contains slash: '%s'", value)
			}
			opts.addSuffix = value
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

// expandSuffixOpts rewrites the short -d and -x options into their long
// forms when they carry an attached starting number, like "-d5", since the
// number may only be attached, never separated.
func expandSuffixOpts(args []string) []string {
	out := make([]string, 0, len(args))
	operands := false
	for _, arg := range args {
		if operands || arg == "" || arg == "-" || arg == "--" || !strings.HasPrefix(arg, "-") {
			operands = true
			out = append(out, arg)
			continue
		}
		if strings.HasPrefix(arg, "--") || !strings.ContainsAny(arg, "dx") {
			out = append(out, arg)
			continue
		}
		body := arg[1:]
		var group []byte
		flush := func() {
			if len(group) > 0 {
				out = append(out, "-"+string(group))
			}
		}
		expanded := false
		for i := 0; i < len(body) && !expanded; i++ {
			c := body[i]
			if strings.ContainsRune("ablnC", rune(c)) {
				// A valued option: the rest of the group is its
				// argument, and the whole option stays as it is.
				out = append(out, arg)
				group = nil
				break
			}
			if c != 'd' && c != 'x' {
				group = append(group, c)
				continue
			}
			flush()
			long := "numeric-suffixes"
			if c == 'x' {
				long = "hex-suffixes"
			}
			if rest := body[i+1:]; rest != "" {
				// Anything attached is the starting number.
				out = append(out, "--"+long+"="+rest)
			} else {
				out = append(out, "--"+long)
			}
			group = nil
			if i+1 < len(body) {
				expanded = true
			}
		}
		if group != nil {
			flush()
		}
	}
	return out
}

// parseChunks parses a -n operand: n, k/n, l/n, l/k/n, r/n or r/k/n.
func parseChunks(value string) (chunkSpec, error) {
	parts := strings.Split(value, "/")
	spec := chunkSpec{mode: 'b'}
	switch parts[0] {
	case "l", "r":
		spec.mode = parts[0][0]
		parts = parts[1:]
	}
	var nStr, kStr string
	switch len(parts) {
	case 1:
		nStr = parts[0]
	case 2:
		kStr, nStr = parts[0], parts[1]
	default:
		return chunkSpec{}, fmt.Errorf("invalid number of chunks: '%s'", value)
	}
	n, err := strconv.ParseInt(nStr, 10, 64)
	if err != nil || n < 1 || n > 1<<31 || !isDecimal(nStr) {
		return chunkSpec{}, fmt.Errorf("invalid number of chunks: '%s'", value)
	}
	spec.n = n
	if kStr != "" {
		k, err := strconv.ParseInt(kStr, 10, 64)
		if err != nil || k < 1 || k > n || !isDecimal(kStr) {
			return chunkSpec{}, fmt.Errorf("invalid chunk number: '%s'", value)
		}
		spec.k = k
	}
	return spec, nil
}

// isDecimal reports whether s holds only decimal digits.
func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// intPow returns base**exp.
func intPow(base, exp int) int64 {
	result := int64(1)
	for range exp {
		result *= int64(base)
	}
	return result
}

// baseName returns the last path element of prefix.
func baseName(prefix string) string {
	return path.Base(filepath.ToSlash(prefix))
}

// readError marks an error as coming from reading the input.
type readError struct {
	err error
}

// Error implements the error interface.
func (e *readError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make readError unwrapable.
func (e *readError) Unwrap() error {
	return e.err
}

// writeError marks an error as coming from writing to an output file.
type writeError struct {
	name string
	err  error
}

// Error implements the error interface.
func (e *writeError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make writeError unwrapable.
func (e *writeError) Unwrap() error {
	return e.err
}

var _ coreutils.Program = (*Prog)(nil)
