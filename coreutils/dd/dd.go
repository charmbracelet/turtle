// Package dd implements the POSIX dd utility.
//
// It copies its input to its output, converting and reblocking the data as
// told by its name=value operands, which are processed in any order, before
// any input is read. The operands are not options: they don't follow the
// utility syntax, so they are parsed here instead of through getopt.
package dd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"strings"
	"time"

	"charm.land/turtle/coreutils"

	"charm.land/turtle/coreutils/internal/ctxio"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// defaultBlock is the input and output block size dd uses when neither bs=
// nor ibs=/obs= are given.
const defaultBlock = 512

// maxBlock caps the block sizes, so they fit the native int of every
// platform and the buffers can still be allocated.
const maxBlock = math.MaxInt32

// Prog is the dd core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run copies the input to the output as told by the operands, writing the
// record counts to the standard error when the copy is done.
//
// Copying is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := p.standardOptions(args); done {
		return err
	}
	op, err := parseOperands(args)
	if err == nil {
		err = op.finalize()
	}
	if err != nil {
		stdopts.Errorf(p.Runtime, "dd", "%v", err)
		stdopts.TryHelp(p.Runtime, "dd")
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return p.run(ctx, op)
}

// run copies the input to the output as told by op.
func (p *Prog) run(ctx context.Context, op *options) error {
	e, err := p.newEngine(ctx, op)
	if err != nil {
		return err
	}
	defer e.close()
	if err = e.loop(ctx); err != nil {
		return err
	}
	if err = e.finish(); err != nil {
		return err
	}
	e.printStats()
	if e.failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// standardOptions handles --help and --version, which are the only options
// dd takes. They are only recognized before the first operand.
func (p *Prog) standardOptions(args []string) (bool, error) {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			return false, nil
		}
		switch arg {
		case "--help":
			if _, err := io.WriteString(p.Stdout, usage); err != nil {
				return true, &coreutils.ExitError{Code: 1, Err: err}
			}
			return true, nil
		case "--version":
			out := stdopts.VersionText("dd")
			if _, err := io.WriteString(p.Stdout, out); err != nil {
				return true, &coreutils.ExitError{Code: 1, Err: err}
			}
			return true, nil
		case "--":
		default:
			err := fmt.Errorf("unrecognized option '%s'", arg)
			stdopts.Errorf(p.Runtime, "dd", "%v", err)
			stdopts.TryHelp(p.Runtime, "dd")
			return true, &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	return false, nil
}

// engine holds the state of one copy: the open input and output, the
// conversion buffers and the record counters.
type engine struct {
	p  *Prog
	op *options

	in      io.Reader
	inFile  fs.File
	inSeek  io.Seeker
	out     io.Writer
	outF    fs.File
	outSeek io.Seeker
	outSync interface{ Sync() error }

	ibuf, sbuf, obuf []byte

	table     [256]byte
	translate bool
	newline   byte
	space     byte

	fullIn, partialIn   int64
	fullOut, partialOut int64
	truncated           int64

	col      int64 // bytes of the record conv=block is filling
	overlong bool  // whether the record conv=block is filling went past cbs
	pending  int   // spaces conv=unblock holds back, which may not be trailing
	oc       int   // bytes buffered in the output block
	saved    int   // byte conv=swab holds back to pair with the next one

	inputOffset int64
	nextStats   time.Time
	failed      bool
}

// newEngine opens the input and output files, skips the blocks the operands
// tell to, and returns the engine of the copy.
func (p *Prog) newEngine(ctx context.Context, op *options) (*engine, error) {
	e := &engine{p: p, op: op, saved: -1}
	e.nextStats = p.Now().Add(time.Second)
	e.ibuf = make([]byte, op.ibs)
	e.obuf = make([]byte, op.obs)
	if op.conv.has(convSwab) {
		e.sbuf = make([]byte, op.ibs+1)
	}
	e.setConversions()

	if err := p.openInput(ctx, e); err != nil {
		return nil, err
	}
	if err := p.openOutput(e); err != nil {
		return nil, err
	}
	if err := e.skipInput(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

// openInput opens the file if= names, or wires the standard input.
func (p *Prog) openInput(ctx context.Context, e *engine) error {
	op := e.op
	if op.input == "" {
		if p.Stdin == nil {
			return p.noStream("no standard input available")
		}
		e.in = ctxio.NewReader(ctx, p.Stdin)
		e.inSeek, _ = p.Stdin.(io.Seeker)
		return nil
	}
	if p.FS == nil {
		return p.noStream("no file system available")
	}
	f, err := p.FS.Open(p.FsName(op.input))
	if err != nil {
		stdopts.Errorf(p.Runtime, "dd", "failed to open '%s': %v", op.input, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	e.inFile = f
	e.in = ctxio.ReadCloser{Ctx: ctx, RC: f}
	e.inSeek, _ = f.(io.Seeker)
	return nil
}

// openOutput opens the file of= names, truncating and seeking it as the
// operands require, or wires the standard output.
func (p *Prog) openOutput(e *engine) error {
	op := e.op
	seekOff := op.seekBlocks*op.obs + op.seekBytes
	if op.output == "" {
		if p.Stdout == nil {
			return p.noStream("no standard output available")
		}
		e.out = p.Stdout
		e.outSeek, _ = p.Stdout.(io.Seeker)
		return e.fillTo(seekOff)
	}
	if p.FS == nil {
		return p.noStream("no file system available")
	}

	// The copy shortens the output file to the seek offset, keeping the
	// blocks it seeks over. The prefix is read before truncating, as the
	// file systems don't share a truncate operation.
	var prefix []byte
	preserve := seekOff > 0 && !op.conv.has(convNotrunc)
	if preserve {
		prefix = e.readPrefix(op.output, seekOff)
	}

	flag := os.O_WRONLY
	if !op.conv.has(convNocreat) {
		flag |= os.O_CREATE
	}
	if op.conv.has(convExcl) {
		flag |= os.O_EXCL
	}
	if op.iflags.has(flagAppend) || op.oflags.has(flagAppend) {
		flag |= os.O_APPEND
	}
	if !op.conv.has(convNotrunc) {
		flag |= os.O_TRUNC
	}
	f, err := p.FS.OpenFile(p.FsName(op.output),
		flag, 0o666&^p.Umask.Perm())
	if err != nil {
		stdopts.Errorf(p.Runtime, "dd", "failed to open '%s': %v", op.output, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	e.outF = f
	e.out = f
	e.outSeek, _ = f.(io.Seeker)
	e.outSync, _ = f.(interface{ Sync() error })

	if preserve {
		return e.rewritePrefix(prefix, seekOff)
	}
	return e.fillTo(seekOff)
}

// readPrefix reads the first n bytes of name, which the copy seeks over and
// preserves. A nil slice means there is nothing to preserve.
func (e *engine) readPrefix(name string, n int64) []byte {
	f, err := e.p.FS.Open(e.p.FsName(name))
	if err != nil {
		return nil
	}
	defer f.Close()
	prefix, err := io.ReadAll(io.LimitReader(f, n))
	if err != nil {
		return nil
	}
	return prefix
}

// rewritePrefix writes prefix back, zero filled up to n, so the output
// file reaches the offset the copy seeks to and holds nothing beyond the
// blocks it preserves.
func (e *engine) rewritePrefix(prefix []byte, n int64) error {
	if _, err := e.out.Write(prefix); err != nil {
		return e.writeError(err)
	}
	return e.writeZeros(n - int64(len(prefix)))
}

// fillTo advances the output to offset n, seeking when it can and writing
// null bytes when it can't, as POSIX requires for non-seekable files.
func (e *engine) fillTo(n int64) error {
	if n <= 0 {
		return nil
	}
	if e.outSeek != nil {
		if _, err := e.outSeek.Seek(n, io.SeekStart); err == nil {
			return nil
		}
	}
	return e.writeZeros(n)
}

// writeZeros writes n null bytes to the output, extending it.
func (e *engine) writeZeros(n int64) error {
	zeros := make([]byte, 32*1024)
	for n > 0 {
		size := min(n, int64(len(zeros)))
		if _, err := e.out.Write(zeros[:size]); err != nil {
			return e.writeError(err)
		}
		n -= size
	}
	return nil
}

// skipInput skips the blocks skip= names, seeking when the input can and
// reading and discarding when it can't. A read error stops the copy, unless
// conv=noerror asks to skip past the bad block and keep going.
func (e *engine) skipInput(ctx context.Context) error {
	op := e.op
	off := op.skipBlocks*op.ibs + op.skipBytes
	if off <= 0 {
		return nil
	}
	if e.inSeek != nil {
		if _, err := e.inSeek.Seek(off, io.SeekStart); err == nil {
			e.inputOffset = off
			return nil
		}
	}
	for off > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		size := min(off, int64(len(e.ibuf)))
		blockStart := e.inputOffset
		n, err := e.in.Read(e.ibuf[:size])
		off -= int64(n)
		e.inputOffset += int64(n)
		if err != nil && !errors.Is(err, io.EOF) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if !op.conv.has(convNoerror) {
				stdopts.Errorf(e.p.Runtime, "dd", "error reading '%s': %v", e.inputLabel(), err)
				e.printStats()
				return &coreutils.ExitError{Code: 1, Err: err}
			}
			if op.status != statusNone {
				stdopts.Errorf(e.p.Runtime, "dd", "error reading '%s': %v", e.inputLabel(), err)
			}
			e.printStats()
			if e.skipFailedBlock(blockStart, int(size)) {
				off -= size - int64(n)
			} else if n == 0 {
				// Without a seek, no progress can be made on this input.
				break
			}
			continue
		}
		if err != nil {
			break
		}
	}
	if off > 0 && op.status != statusNone {
		stdopts.Errorf(e.p.Runtime, "dd", "'%s': cannot skip to specified offset", e.inputLabel())
	}
	return nil
}

// loop reads the input block by block, converting and writing each block,
// until the end of the input or the count= limit.
func (e *engine) loop(ctx context.Context) error {
	op := e.op
	pad := byte(0)
	if op.conv.hasAny(convBlock, convUnblock) {
		pad = ' '
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		e.progress()

		size := int(op.ibs)
		if op.countSet {
			total := e.fullIn + e.partialIn
			if total >= op.countBlocks &&
				(op.countBytes == 0 || total > op.countBlocks) {
				return nil
			}
			if total >= op.countBlocks {
				size = int(op.countBytes)
			}
		}

		// With conv=sync,noerror, the buffer is filled ahead of the read, so
		// the data before an error is kept and the rest zeroed.
		if op.conv.has(convSync) && op.conv.has(convNoerror) {
			for i := range e.ibuf {
				e.ibuf[i] = pad
			}
		}

		blockStart := e.inputOffset
		n, err := e.read(ctx, size)
		e.inputOffset += int64(n)
		zero := false
		if err != nil && !errors.Is(err, io.EOF) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if !op.conv.has(convNoerror) {
				stdopts.Errorf(e.p.Runtime, "dd", "error reading '%s': %v", e.inputLabel(), err)
				e.failed = true
				break
			}
			if op.status != statusNone {
				stdopts.Errorf(e.p.Runtime, "dd", "error reading '%s': %v", e.inputLabel(), err)
			}
			e.printStats()
			// The failed read leaves the rest of its block unread, which is
			// skipped past: what the read did return may be part of the block.
			// When the input can't be repositioned and the read returned
			// nothing, the failed block stays in the way and the copy can't go
			// on.
			if !e.skipFailedBlock(blockStart, size) && n == 0 {
				e.failed = true
				break
			}
			if op.conv.has(convSync) {
				// The zeroed block, with whatever was read before the error,
				// replaces the missing input.
				zero = true
			} else {
				continue
			}
		}
		if n == 0 && err != nil && !zero {
			break // EOF
		}
		if n == 0 && err == nil && !zero {
			continue
		}

		if n < int(op.ibs) {
			e.partialIn++
			if op.conv.has(convSync) {
				for i := n; i < int(op.ibs); i++ {
					e.ibuf[i] = pad
				}
				n = int(op.ibs)
			}
		} else {
			e.fullIn++
		}

		var werr error
		if op.direct() {
			werr = e.writeBlock(e.ibuf[:n])
		} else {
			werr = e.process(e.ibuf[:n])
		}
		if werr != nil {
			return werr
		}
	}
	return nil
}

// read reads one input block, of at most size bytes, accumulating reads to
// fill the block when iflag=fullblock is given.
func (e *engine) read(ctx context.Context, size int) (int, error) {
	n := 0
	for n < size {
		nn, err := e.in.Read(e.ibuf[n:size])
		n += nn
		if err != nil {
			return n, err
		}
		if !e.op.iflags.has(flagFullblock) {
			break
		}
	}
	return n, nil
}

// skipFailedBlock skips the rest of the input block a read failed on,
// reporting whether the input could be repositioned past it. The read may
// have returned some of the bytes along with the error, and failed reads on
// regular files leave the rest unread.
func (e *engine) skipFailedBlock(blockStart int64, size int) bool {
	if e.inSeek == nil {
		return false
	}
	target := blockStart + int64(size)
	if _, err := e.inSeek.Seek(target, io.SeekStart); err != nil {
		stdopts.Errorf(e.p.Runtime, "dd", "'%s': cannot seek", e.inputLabel())
		return false
	}
	e.inputOffset = target
	return true
}

// finish completes the copy: the byte conv=swab held back, the final record
// of conv=block and conv=unblock, and the last partial output block.
func (e *engine) finish() error {
	op := e.op
	if e.saved >= 0 {
		// The byte conv=swab held back was never swapped, but it does go
		// through the remaining conversions.
		c := byte(e.saved)
		if e.translate {
			c = e.table[c]
		}
		last := []byte{c}
		var err error
		switch {
		case op.conv.has(convBlock):
			err = e.convertBlock(last)
		case op.conv.has(convUnblock):
			err = e.convertUnblock(last)
		default:
			err = e.convertSimple(last)
		}
		if err != nil {
			return err
		}
	}
	if op.conv.has(convBlock) && e.col > 0 {
		for ; e.col < op.cbs; e.col++ {
			if err := e.outputByte(e.space); err != nil {
				return err
			}
		}
	}
	if op.conv.has(convUnblock) && e.col != 0 {
		if err := e.outputByte(e.newline); err != nil {
			return err
		}
	}
	if err := e.flush(); err != nil {
		return err
	}
	if op.conv.has(convFsync) && e.outSync != nil {
		if err := e.outSync.Sync(); err != nil {
			stdopts.Errorf(e.p.Runtime, "dd", "error syncing '%s': %v", e.outputLabel(), err)
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	return nil
}

// writeBlock writes one whole output block, as bs= copies do.
func (e *engine) writeBlock(data []byte) error {
	n, err := e.out.Write(data)
	if err != nil || n != len(data) {
		if err == nil {
			err = errShortWrite
		}
		stdopts.Errorf(e.p.Runtime, "dd", "error writing '%s': %v", e.outputLabel(), err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if n == int(e.op.obs) {
		e.fullOut++
	} else {
		e.partialOut++
	}
	return nil
}

// convertSimple aggregates the converted data into output blocks.
func (e *engine) convertSimple(data []byte) error {
	for len(data) > 0 {
		n := min(len(data), len(e.obuf)-e.oc)
		copy(e.obuf[e.oc:], data[:n])
		e.oc += n
		data = data[n:]
		if e.oc == len(e.obuf) {
			if err := e.flush(); err != nil {
				return err
			}
		}
	}
	return nil
}

// outputByte buffers one converted byte, writing the output block when it
// fills up.
func (e *engine) outputByte(c byte) error {
	e.obuf[e.oc] = c
	e.oc++
	if e.oc == len(e.obuf) {
		return e.flush()
	}
	return nil
}

// flush writes the output block buffered so far.
func (e *engine) flush() error {
	if e.oc == 0 {
		return nil
	}
	n, err := e.out.Write(e.obuf[:e.oc])
	if err != nil || n != e.oc {
		if err == nil {
			err = errShortWrite
		}
		stdopts.Errorf(e.p.Runtime, "dd", "error writing '%s': %v", e.outputLabel(), err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if e.oc == len(e.obuf) {
		e.fullOut++
	} else {
		e.partialOut++
	}
	e.oc = 0
	return nil
}

// progress prints the record counts once a second, as status=progress asks.
func (e *engine) progress() {
	if e.op.status != statusProgress {
		return
	}
	now := e.p.Now()
	if now.Before(e.nextStats) {
		return
	}
	e.printStats()
	e.nextStats = now.Add(time.Second)
}

// printStats writes the record counts to the standard error.
func (e *engine) printStats() {
	if e.op.status == statusNone {
		return
	}
	fmt.Fprintf(e.p.Stderr, "%d+%d records in\n%d+%d records out\n",
		e.fullIn, e.partialIn, e.fullOut, e.partialOut)
	if e.truncated != 0 {
		fmt.Fprintf(e.p.Stderr, "%d truncated record%s\n",
			e.truncated, plural(e.truncated))
	}
}

func (e *engine) inputLabel() string {
	if e.op.input == "" {
		return "standard input"
	}
	return e.op.input
}

func (e *engine) outputLabel() string {
	if e.op.output == "" {
		return "standard output"
	}
	return e.op.output
}

func (e *engine) close() {
	if e.inFile != nil {
		_ = e.inFile.Close()
	}
	if e.outF != nil {
		_ = e.outF.Close()
	}
}

// writeError reports a failure writing the output.
func (e *engine) writeError(err error) error {
	stdopts.Errorf(e.p.Runtime, "dd", "error writing '%s': %v", e.outputLabel(), err)
	return &coreutils.ExitError{Code: 1, Err: err}
}

func (p *Prog) noStream(msg string) error {
	stdopts.Errorf(p.Runtime, "dd", "%s", msg)
	return &coreutils.ExitError{Code: 1, Err: errors.New(msg)}
}

var errShortWrite = errors.New("short write")

func plural(n int64) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// usage describes the operands of dd.
const usage = `Usage: dd [OPERAND]...
  or:  dd OPTION
Copy a file, converting and formatting according to the operands.

  bs=BYTES
         read and write up to BYTES bytes at a time (default: 512);
         overrides ibs and obs
  cbs=BYTES
         convert BYTES bytes at a time
  conv=CONVS
         convert the file as per the comma separated symbol list
  count=N
         copy only N input blocks
  ibs=BYTES
         read up to BYTES bytes at a time (default: 512)
  if=FILE
         read from FILE instead of standard input
  iflag=FLAGS
         read as per the comma separated symbol list
  obs=BYTES
         write BYTES bytes at a time (default: 512)
  of=FILE
         write to FILE instead of standard output
  oflag=FLAGS
         write as per the comma separated symbol list
  seek=N
         (or oseek=N) skip N obs-sized output blocks
  skip=N
         (or iseek=N) skip N ibs-sized input blocks
  status=LEVEL
         the LEVEL of information to print to stderr:
         'none' suppresses everything but error messages, 'noxfer'
         suppresses the final transfer statistics, 'progress' shows
         periodic transfer statistics

N and BYTES may be followed by the following multiplicative suffixes:
c=1, w=2, b=512, kB=1000, K=1024, MB=1000*1000, M=1024*1024, xM=M,
GB=1000*1000*1000, G=1024*1024*1024, and so on for T, P, E, Z, Y, R, Q.
Binary prefixes can be used, too: KiB=K, MiB=M, and so on.
If N ends in 'B', it counts bytes not blocks.

Each CONV symbol may be:

  ascii     from EBCDIC to ASCII
  ebcdic    from ASCII to EBCDIC
  ibm       from ASCII to alternate EBCDIC
  block     pad newline-terminated records with spaces to cbs-size
  unblock   replace trailing spaces in cbs-size records with newline
  lcase     change upper case to lower case
  ucase     change lower case to upper case
  swab      swap every pair of input bytes
  sync      pad every input block with NULs to ibs-size; when used
            with block or unblock, pad with spaces rather than NULs
  excl      fail if the output file already exists
  nocreat   do not create the output file
  notrunc   do not truncate the output file
  noerror   continue after read errors
  fsync     physically write output file data before finishing

Each FLAG symbol may be:

  append    append mode (makes sense only for output; conv=notrunc
            suggested)
  fullblock  accumulate full blocks of input (iflag only)

` + stdopts.Tail
