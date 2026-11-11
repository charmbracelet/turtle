// Package tail implements the tail utility.
//
// It copies the last part of each given file to the standard output, starting
// at the point given by the -n or -c options, defaulting to the last 10
// lines. When more than one operand is given, a "==> name <==" header
// precedes the output of each file; -q and -v make the headers never or
// always printed. The standard input is used when no operand is given, and
// also when the operand is "-". With -f, or --follow=name with --retry, as
// with -F, the inputs keep being copied as further data becomes available:
// by descriptor, which keeps reading the open files, or by name, which also
// reopens them when replaced or removed, reporting truncations and printing
// a header whenever the output comes from a different file. The --pid, -s
// and --max-unchanged-stats options tune the waiting: tail exits once all
// the given processes no longer exist, waits a given number of seconds
// between iterations, and rechecks a name after a given number of unchanged
// ones. The obsolete first-operand forms tail -NUM and tail +NUM, with an
// optional b, c or l size letter and a trailing f, are accepted too.
package tail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/sizeparse"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// defaultSleep is how long the follow modes wait between iterations, like
// the loop described on the POSIX rationale.
const defaultSleep = time.Second

// defaultMaxUnchanged is how many iterations without a size change happen
// before a followed name is checked again.
const defaultMaxUnchanged = 5

// Prog is the tail core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the designated part of the given files to the standard output,
// in the same order the operands are given. The standard input is used when
// no operands are given, and also when an operand is "-". With -f, the
// inputs keep being copied as further data becomes available, and Run only
// returns once ctx is done, all the writers given with --pid no longer
// exist, or every followed name is given up. A failing file doesn't stop
// the others.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "tail", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "tail")
		return &coreutils.ExitError{Code: 2}
	}
	inputs := input.List(operands)
	if opts.follow {
		return p.runFollow(ctx, inputs, opts)
	}
	if opts.retry {
		p.errorf("warning: --retry ignored; --retry is useful only when following")
	}
	if len(opts.pids) > 0 {
		p.errorf("warning: PID ignored; --pid=PID is useful only when following")
	}
	return p.copyInputs(ctx, inputs, opts)
}

// copyInputs writes the designated part of each input to the standard
// output, in the order the operands were given. A failing input doesn't
// stop the others.
func (p *Prog) copyInputs(ctx context.Context, inputs []input.Input, opts options) error {
	headers := opts.headers == headerAlways ||
		(opts.headers == headerAuto && len(inputs) > 1)

	ok := true
	first := true
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !in.Stdin && in.Name == "" {
			p.errorf("invalid zero-length file name")
			ok = false
			continue
		}
		rc, f, err := p.open(in)
		if err != nil {
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
			ok = false
			continue
		}
		if headers {
			p.writeHeader(in, !first)
			first = false
		}
		err = p.copyPortion(ctxio.NewReader(ctx, rc), f, opts)
		rc.Close()
		if err == nil {
			continue
		}
		var writeErr *writeError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 1, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// runFollow writes the designated part of each input, and then keeps
// copying further data from the followed ones as it becomes available.
func (p *Prog) runFollow(ctx context.Context, inputs []input.Input, opts options) error {
	// The standard input of a FIFO or a pipe can't be followed: -f is
	// ignored when no file operand is given, and a "-" operand isn't
	// followed likewise.
	stdinPipe := stdinIsPipe(p.Runtime)
	if stdinPipe && len(inputs) == 1 && inputs[0].Stdin && inputs[0].Name == "" {
		return p.copyInputs(ctx, inputs, opts)
	}
	if opts.retry && !opts.byName {
		p.errorf("warning: --retry only effective for the initial open")
	}
	if len(opts.pids) > 0 && p.ProcessAlive == nil {
		p.errorf("warning: --pid=PID is not supported on this system")
		opts.pids = nil
	}

	headers := opts.headers == headerAlways ||
		(opts.headers == headerAuto && len(inputs) > 1)

	ok := true
	following := false
	out := &output{p: p, headers: headers}
	var fls []*follower
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !in.Stdin && in.Name == "" {
			p.errorf("invalid zero-length file name")
			ok = false
			continue
		}
		follow := !in.Stdin || !stdinPipe
		if in.Stdin && in.Name == "-" && follow && opts.byName {
			p.errorf("cannot follow '-' by name")
			return &coreutils.ExitError{Code: 1}
		}
		following = following || follow

		rc, f, err := p.openRetry(ctx, in, opts)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			ok = false
			continue
		}
		fl := &follower{
			p:            p,
			in:           in,
			ctx:          ctx,
			rc:           ctxio.ReadCloser{Ctx: ctx, RC: rc},
			f:            f,
			out:          out,
			path:         p.FsName(in.Name),
			byName:       opts.byName && !in.Stdin,
			retry:        opts.retry,
			sleep:        opts.sleep,
			maxUnchanged: opts.maxUnchanged,
		}
		if headers {
			if follow {
				out.header(fl)
			} else {
				out.headerIn(in)
			}
		}
		if err := p.copyPortion(fl.rc, f, opts); err != nil {
			fl.close()
			var writeErr *writeError
			switch {
			case errors.As(err, &writeErr):
				p.errorf("write error: %v", writeErr.err)
				return &coreutils.ExitError{Code: 1, Err: writeErr.err}
			case ctx.Err() != nil:
				return ctx.Err()
			default:
				p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
				ok = false
			}
			continue
		}
		if f != nil {
			if pos, err := f.Seek(0, io.SeekCurrent); err == nil {
				fl.offset = pos
			}
			if fi, err := f.Stat(); err == nil {
				fl.stat, fl.lastSize = fi, fi.Size()
			}
		}
		if follow {
			fls = append(fls, fl)
		} else {
			fl.close()
		}
	}
	if len(fls) == 0 {
		if following {
			p.errorf("no files remaining")
			return &coreutils.ExitError{Code: 1}
		}
		if !ok {
			return &coreutils.ExitError{Code: 1}
		}
		return nil
	}
	if opts.debug {
		// There is no notification support, so the follow always polls.
		p.errorf("using polling mode")
	}
	return p.followAll(ctx, fls, opts, ok)
}

// followAll keeps copying further data from every follower, until ctx is
// done, all the writers given with --pid no longer exist, or every followed
// name is given up. ok tells whether errors were already reported.
func (p *Prog) followAll(ctx context.Context, fls []*follower, opts options, ok bool) error {
	stop := make(chan struct{})
	halt := sync.OnceFunc(func() { close(stop) })
	results := make(chan followResult, 2)

	var files, pipes []*follower
	for _, fl := range fls {
		if fl.in.Stdin {
			pipes = append(pipes, fl)
		} else {
			files = append(files, fl)
		}
	}

	// The files are polled once per iteration, in the order their operands
	// were given. The standard input is followed on its own, as reading it
	// blocks until data arrives.
	var loops, watchers sync.WaitGroup
	loops.Add(2)
	go func() {
		defer loops.Done()
		results <- p.pollLoop(ctx, files, opts, stop, halt)
	}()
	go func() {
		defer loops.Done()
		results <- p.followPipes(ctx, pipes, opts, stop, halt)
	}()
	if len(opts.pids) > 0 {
		watchers.Go(func() {
			p.watchPids(ctx, opts, stop, halt)
		})
	}
	// Once nothing is followed anymore, the writer watcher is stopped too.
	go func() {
		loops.Wait()
		halt()
	}()
	watchers.Wait()
	loops.Wait()
	halt()

	var (
		writeErr *writeError
		gaveUp   bool
	)
	for range 2 {
		res := <-results
		gaveUp = gaveUp || res.gaveUp
		if res.err != nil {
			errors.As(res.err, &writeErr)
		}
	}
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case writeErr != nil:
		p.errorf("write error: %v", writeErr.err)
		return &coreutils.ExitError{Code: 1, Err: writeErr.err}
	case gaveUp:
		p.errorf("no files remaining")
		return &coreutils.ExitError{Code: 1}
	case !ok:
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// followResult is how a follow loop ended: whether a followed name was
// given up, and the error which makes the whole run stop, if any.
type followResult struct {
	gaveUp bool
	err    error
}

// pollLoop polls the followed files once per iteration, in the order their
// operands were given, until they are all given up, stop is closed, or ctx
// is done.
func (p *Prog) pollLoop(ctx context.Context, files []*follower, opts options, stop <-chan struct{}, halt func()) followResult {
	gaveUp := false
	for len(files) > 0 {
		select {
		case <-stop:
			return followResult{}
		default:
		}
		if err := ctx.Err(); err != nil {
			return followResult{err: err}
		}
		var next []*follower
		for _, fl := range files {
			ended, err := fl.poll(ctx)
			if err != nil {
				halt()
				return followResult{err: err}
			}
			if ended {
				gaveUp = true
				fl.close()
				continue
			}
			next = append(next, fl)
		}
		files = next
		if len(files) == 0 {
			break
		}
		if err := p.Wait(ctx, opts.sleep); err != nil {
			return followResult{err: err}
		}
	}
	return followResult{gaveUp: gaveUp}
}

// followPipes follows the standard-input operands, whose reads block until
// data arrives.
func (p *Prog) followPipes(ctx context.Context, pipes []*follower, opts options, stop <-chan struct{}, halt func()) followResult {
	if len(pipes) == 0 {
		return followResult{}
	}
	gaveUp := false
	for len(pipes) > 0 {
		select {
		case <-stop:
			return followResult{}
		default:
		}
		if err := ctx.Err(); err != nil {
			return followResult{err: err}
		}
		var next []*follower
		for _, fl := range pipes {
			ended, err := fl.poll(ctx)
			if err != nil {
				halt()
				return followResult{err: err, gaveUp: gaveUp}
			}
			if ended {
				gaveUp = true
				fl.close()
				continue
			}
			next = append(next, fl)
		}
		pipes = next
		if len(pipes) == 0 {
			break
		}
		if err := p.Wait(ctx, opts.sleep); err != nil {
			return followResult{err: err, gaveUp: gaveUp}
		}
	}
	return followResult{gaveUp: gaveUp}
}

// watchPids watches the processes given with --pid, stopping the follow once
// none of them exists anymore.
func (p *Prog) watchPids(ctx context.Context, opts options, stop <-chan struct{}, halt func()) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		if p.pidsDead(opts.pids) {
			halt()
			return
		}
		if err := p.Wait(ctx, opts.sleep); err != nil {
			return
		}
	}
}

// pidsDead reports whether none of the given processes exists anymore.
func (p *Prog) pidsDead(pids []int) bool {
	return !slices.ContainsFunc(pids, p.ProcessAlive)
}

// follower is one input being followed after its designated part was copied.
type follower struct {
	p    *Prog
	in   input.Input
	ctx  context.Context
	rc   ctxio.ReadCloser // the open input, nil while the name is inaccessible
	f    fsx.SeekFile     // the open file when it can be seeked, nil otherwise
	out  *output
	path string // the resolved name to reopen, empty for the standard input

	byName       bool  // --follow=name: track the name instead of the descriptor
	retry        bool  // --retry: keep waiting for an inaccessible name
	offset       int64 // position already written for the current file
	lastSize     int64 // size seen on the previous iteration
	unchanged    int   // iterations without a size change since then
	maxUnchanged int   // iterations before the name is checked again
	stat         fs.FileInfo
	sleep        time.Duration
	buf          []byte
}

// poll copies the data available on the followed input, and checks whether
// the file was truncated or, when following by name, replaced or removed. It
// reports whether the input is still being followed; a non-nil error is
// fatal for the whole run.
func (fl *follower) poll(ctx context.Context) (ended bool, err error) {
	if fl.rc.RC == nil && !fl.reappeared() {
		return false, nil
	}
	for {
		ended, err := fl.drain(ctx)
		if ended || err != nil {
			return ended, err
		}
		if again, ended := fl.recheck(); ended {
			return true, nil
		} else if !again {
			return false, nil
		}
	}
}

// reappeared reopens an inaccessible name which exists again, reporting
// whether it did. A file which is a new one is followed from its beginning.
func (fl *follower) reappeared() bool {
	fi, err := fs.Stat(fl.p.FS, fl.path)
	if err != nil {
		return false
	}
	fresh := !sameFile(fi, fl.stat)
	if fresh {
		fl.p.errorf("'%s' has appeared;  following new file", fl.in.Label())
	}
	if err := fl.reopen(fresh); err != nil {
		return fl.inaccessible(err)
	}
	return true
}

// drain copies the data available on the open input, reporting read errors,
// which end the following of this input.
func (fl *follower) drain(ctx context.Context) (bool, error) {
	if fl.buf == nil {
		fl.buf = make([]byte, 32*1024)
	}
	for {
		n, err := fl.rc.Read(fl.buf)
		if n > 0 {
			if werr := fl.out.data(fl, fl.buf[:n]); werr != nil {
				return false, werr
			}
			fl.offset += int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			fl.p.errorf("%s: %v", fl.in.Label(), patherr.Unwrap(err))
			return true, nil
		}
	}
}

// recheck looks for a truncation, and, when following by name, for a
// replacement of the followed file. It reports whether the input must be
// drained again, and whether the name was given up.
func (fl *follower) recheck() (drainAgain, ended bool) {
	if fl.f == nil {
		return false, false
	}
	fi, err := fl.f.Stat()
	if err != nil {
		fl.p.errorf("%s: %v", fl.in.Label(), patherr.Unwrap(err))
		return false, true
	}
	if fi.Size() < fl.offset {
		fl.p.errorf("%s: file truncated", fl.in.Label())
		if _, err := fl.f.Seek(0, io.SeekStart); err != nil {
			fl.p.errorf("%s: %v", fl.in.Label(), patherr.Unwrap(err))
			return false, true
		}
		fl.offset = 0
		fl.stat = fi
		return true, false
	}
	if !fl.byName {
		return false, false
	}
	if fi.Size() != fl.lastSize {
		fl.lastSize = fi.Size()
		fl.unchanged = 0
		return false, false
	}
	fl.unchanged++
	if fl.unchanged < fl.maxUnchanged {
		return false, false
	}
	fl.unchanged = 0
	return fl.recheckName()
}

// recheckName checks whether the followed name still designates the file
// being read, as the GNU manual describes for --max-unchanged-stats.
func (fl *follower) recheckName() (drainAgain, ended bool) {
	fi, err := fs.Stat(fl.p.FS, fl.path)
	switch {
	case err != nil:
		if !fl.retry {
			fl.p.errorf("'%s' has become inaccessible: %v; giving up on this name", fl.in.Label(), patherr.Unwrap(err))
			fl.close()
			return false, true
		}
		fl.p.errorf("'%s' has become inaccessible: %v", fl.in.Label(), patherr.Unwrap(err))
		fl.close()
		return false, false
	case sameFile(fi, fl.stat):
		return false, false
	}
	fl.p.errorf("'%s' has been replaced;  following new file", fl.in.Label())
	if err := fl.reopen(true); err != nil {
		return false, fl.inaccessible(err)
	}
	return true, false
}

// reopen replaces the open input with a fresh open of the name, starting
// over from its beginning when the file is a new one.
func (fl *follower) reopen(fresh bool) error {
	fl.close()
	f, err := fl.p.FS.Open(fl.path)
	if err != nil {
		return err
	}
	fl.rc = ctxio.ReadCloser{Ctx: fl.ctx, RC: f}
	if s, ok := f.(fsx.SeekFile); ok {
		fl.f = s
		if fi, err := s.Stat(); err == nil {
			fl.stat, fl.lastSize = fi, fi.Size()
		}
		if fresh {
			fl.offset = 0
		}
		if _, err := s.Seek(fl.offset, io.SeekStart); err != nil {
			return err
		}
	} else {
		fl.f = nil
	}
	fl.unchanged = 0
	return nil
}

// inaccessible reports the failure to reach the followed name, giving up on
// it without --retry.
func (fl *follower) inaccessible(err error) bool {
	if !fl.retry {
		fl.p.errorf("'%s' has become inaccessible: %v; giving up on this name", fl.in.Label(), patherr.Unwrap(err))
		return true
	}
	fl.p.errorf("'%s' has become inaccessible: %v", fl.in.Label(), patherr.Unwrap(err))
	return false
}

// close closes the open input, if any.
func (fl *follower) close() {
	if fl.rc.RC != nil {
		fl.rc.Close()
	}
	fl.rc = ctxio.ReadCloser{}
	fl.f = nil
}

// openRetry opens an input, retrying while --retry is given. Only the first
// failure is reported, and a file which appears is announced as a new one.
func (p *Prog) openRetry(ctx context.Context, in input.Input, opts options) (io.ReadCloser, fsx.SeekFile, error) {
	rc, f, err := p.open(in)
	if err == nil {
		return rc, f, nil
	}
	p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
	if !opts.retry {
		return nil, nil, err
	}
	for {
		if werr := p.Wait(ctx, opts.sleep); werr != nil {
			return nil, nil, werr
		}
		if rc, f, err = p.open(in); err == nil {
			p.errorf("'%s' has appeared;  following new file", in.Label())
			return rc, f, nil
		}
	}
}

// output writes to the standard output of a follow run, printing a file
// header whenever the output switches from one input to another.
type output struct {
	p       *Prog
	mu      sync.Mutex
	headers bool
	printed bool
	last    *follower
}

// header writes the header of the input of fl, and records it as the last
// one written.
func (o *output) header(fl *follower) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.headerLocked(fl.in)
	o.last = fl
}

// headerIn writes the header of an input which won't produce further output.
func (o *output) headerIn(in input.Input) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.headerLocked(in)
	o.last = nil
}

// data writes data produced by fl, with its header when the output switches
// to it.
func (o *output) data(fl *follower, data []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.headers && o.last != fl {
		o.headerLocked(fl.in)
	}
	o.last = fl
	return o.p.writeBytes(data)
}

// headerLocked writes the "==> name <==" header which precedes the output
// of each file, with a leading blank line after the first one. The caller
// must hold mu.
func (o *output) headerLocked(in input.Input) {
	if o.printed {
		fmt.Fprint(o.p.Stdout, "\n")
	}
	o.printed = true
	name := in.Label()
	if in.Stdin {
		name = "standard input"
	}
	fmt.Fprintf(o.p.Stdout, "==> %s <==\n", name)
}

// copyPortion copies the part of the input selected by the options.
func (p *Prog) copyPortion(r io.Reader, f fsx.SeekFile, opts options) error {
	delim := opts.delim()
	switch {
	case opts.reverse:
		return p.reverse(r, opts)
	case opts.lines && opts.fromStart:
		return p.copyFromLine(r, opts.count, delim)
	case opts.lines:
		return p.copyLastLines(r, f, opts.count, delim)
	case opts.fromStart:
		return p.copyFromByte(r, f, opts.count)
	default:
		return p.copyLastBytes(r, f, opts.count)
	}
}

// writeHeader writes the "==> name <==" header which precedes the output of
// each file. The first one has no leading <newline>; the others are
// separated from the output of the previous file by a blank line.
func (p *Prog) writeHeader(in input.Input, leadingNewline bool) {
	if leadingNewline {
		fmt.Fprint(p.Stdout, "\n")
	}
	name := in.Label()
	if in.Stdin {
		name = "standard input"
	}
	fmt.Fprintf(p.Stdout, "==> %s <==\n", name)
}

// copyLastBytes copies the last count bytes, seeking when the input is
// seekable and buffering otherwise.
func (p *Prog) copyLastBytes(r io.Reader, f fsx.SeekFile, count int) error {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return err
	}
	if f != nil {
		if err := seekLastBytes(f, count); err != nil {
			return err
		}
		return p.copyAll(r)
	}
	data, err := readLastBytes(r, count)
	if err != nil {
		return err
	}
	return p.writeBytes(data)
}

// copyFromByte copies everything from the count'th byte on. The origin is 1.
func (p *Prog) copyFromByte(r io.Reader, f fsx.SeekFile, count int) error {
	if f != nil {
		if _, err := f.Seek(int64(count-1), io.SeekStart); err != nil {
			return err
		}
	} else if _, err := io.CopyN(io.Discard, r, int64(count-1)); err != nil &&
		!errors.Is(err, io.EOF) {
		return err
	}
	return p.copyAll(r)
}

// copyLastLines copies the last count lines, seeking backwards when the
// input is seekable and buffering otherwise. A final line without a trailing
// <newline> counts as a line.
func (p *Prog) copyLastLines(r io.Reader, f fsx.SeekFile, count int, delim byte) error {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return err
	}
	if f != nil {
		if err := seekLastLines(f, count, delim); err != nil {
			return err
		}
		return p.copyAll(r)
	}
	lines, err := readLastLines(r, count, delim)
	if err != nil {
		return err
	}
	return p.writeLines(lines)
}

// copyFromLine copies everything from the count'th line on. The origin is 1.
func (p *Prog) copyFromLine(r io.Reader, count int, delim byte) error {
	br := bufio.NewReader(r)
	for i := 0; i < count-1; i++ {
		if _, err := br.ReadBytes(delim); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
	return p.copyAll(br)
}

// reverse writes the lines of the input in reverse order, last line first,
// concatenated as they are read. With -n, only the last count lines are
// written.
func (p *Prog) reverse(r io.Reader, opts options) error {
	var (
		lines [][]byte
		err   error
	)
	if opts.countSet {
		lines, err = readLastLines(r, opts.count, opts.delim())
	} else {
		lines, err = readAllLines(r, opts.delim())
	}
	if err != nil {
		return err
	}
	for _, line := range slices.Backward(lines) {
		if werr := p.writeBytes(line); werr != nil {
			return werr
		}
	}
	return nil
}

// copyAll copies everything left on r to the standard output.
func (p *Prog) copyAll(r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if werr := p.writeBytes(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// writeBytes writes data to the standard output, wrapping write errors.
func (p *Prog) writeBytes(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if _, err := p.Stdout.Write(data); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// writeLines writes each line to the standard output.
func (p *Prog) writeLines(lines [][]byte) error {
	for _, line := range lines {
		if err := p.writeBytes(line); err != nil {
			return err
		}
	}
	return nil
}

// open returns a reader for the input, along with the file when it's
// seekable: reading from the end then avoids buffering the whole input.
func (p *Prog) open(in input.Input) (io.ReadCloser, fsx.SeekFile, error) {
	if in.Stdin {
		if p.Stdin == nil {
			return nil, nil, errors.New("no standard input available")
		}
		return io.NopCloser(p.Stdin), nil, nil
	}
	if p.FS == nil {
		return nil, nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(p.FsName(in.Name))
	if err != nil {
		return nil, nil, err
	}
	var seek fsx.SeekFile
	if s, ok := f.(fsx.SeekFile); ok {
		seek = s
	}
	return f, seek, nil
}

// stdinIsPipe reports whether the standard input is a FIFO or a pipe, which
// the follow modes can't read further data from.
func stdinIsPipe(rt *coreutils.Runtime) bool {
	s, ok := rt.Stdin.(interface{ Stat() (fs.FileInfo, error) })
	if !ok {
		return false
	}
	fi, err := s.Stat()
	return err == nil && fi.Mode()&fs.ModeNamedPipe != 0
}

// sameFile reports whether two file informations describe the same file,
// comparing the device and inode numbers of the real file systems, and the
// entries of the memory one. Files without identity information always
// compare as the same one.
func sameFile(a, b fs.FileInfo) bool {
	if a == nil || b == nil {
		return false
	}
	if os.SameFile(a, b) {
		return true
	}
	ma, aok := a.Sys().(*fsx.MemFile)
	mb, bok := b.Sys().(*fsx.MemFile)
	return aok && bok && ma == mb
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "tail: "+format+"\n", args...)
}

// The pseudo-options stand for the long options of tail which have no short
// form, as a short argument can't hold their control characters.
const (
	retryOpt        rune = 0
	pidOpt          rune = 1
	maxUnchangedOpt rune = 2
	debugOpt        rune = 3
)

// longOptions maps the long options of tail to the short ones they stand
// for. The -r option has no long form in GNU.
var longOptions = map[string]rune{
	"bytes":               'c',
	"debug":               debugOpt,
	"follow":              'f',
	"lines":               'n',
	"max-unchanged-stats": maxUnchangedOpt,
	"pid":                 pidOpt,
	"quiet":               'q',
	"retry":               retryOpt,
	"silent":              'q',
	"sleep-interval":      's',
	"verbose":             'v',
	"zero-terminated":     'z',
}

// optSet describes the options of tail.
var optSet = getopt.Set{
	Valued:   "cns" + string(pidOpt) + string(maxUnchangedOpt),
	Attached: "f",
	Long:     longOptions,
}

// usage describes the options of tail.
const usage = `Usage: tail [OPTION]... [FILE]...
Print the last 10 lines of each FILE to standard output.
With more than one FILE, precede each with a header giving the file name.

With no FILE, or when FILE is -, read standard input.

  -c, --bytes=[+]NUM
         output the last NUM bytes; or use -c +NUM to output starting with
         byte NUM of each file
  -f, --follow[={name|descriptor}]
         output appended data as the file grows; an absent option argument
         means 'descriptor'
  -F
         same as --follow=name --retry
      --max-unchanged-stats=N
         with --follow=name, reopen a FILE which has not changed size after
         N (default 5) iterations to see if it has been unlinked or renamed
         (this is a common workaround for log rotation)
  -n, --lines=[+]NUM
         output the last NUM lines, instead of the last 10; or use -n +NUM
         to output starting with line NUM
      --pid=PID
         with -f, terminate after process ID, PID dies
  -q, --quiet, --silent
         never print headers giving file names
      --retry
         keep trying to open a file if it is inaccessible
  -r
         output the lines in reverse, starting from the end
  -s, --sleep-interval=N
         with -f, sleep for approximately N seconds (default 1.0) between
         iterations; with --pid=P, check process P at least once every N
         seconds
  -v, --verbose
         always print headers giving file names
  -z, --zero-terminated
         line delimiter is NUL, not newline
      --help
         display this help and exit
      --version
         output version information and exit
`

func parseFlags(args []string) (options, []string, error) {
	var opts options
	if len(args) > 0 {
		if obsolete, rest, ok := parseObsolete(args); ok {
			opts, args = obsolete, rest
		}
	}
	opts.sleep = defaultSleep
	opts.maxUnchanged = defaultMaxUnchanged
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'f':
			opts.follow = true
			switch value {
			case "", "descriptor":
			case "name":
				opts.byName = true
			default:
				return fmt.Errorf("invalid argument '%s' for '--follow'", value)
			}
		case 'F':
			opts.follow = true
			opts.byName = true
			opts.retry = true
		case 'r':
			opts.reverse = true
		case 'q':
			opts.headers = headerNever
		case 'v':
			opts.headers = headerAlways
		case 'z':
			opts.zero = true
		case 's':
			d, err := parseSeconds(value)
			if err != nil {
				return err
			}
			opts.sleep = d
		case 'c', 'n':
			count, fromStart, err := parseCount(value)
			if err != nil {
				return err
			}
			opts.lines = opt == 'n'
			opts.countSet = true
			opts.fromStart = fromStart
			opts.count = count
		case retryOpt:
			opts.retry = true
		case debugOpt:
			opts.debug = true
		case pidOpt:
			pid, err := parsePID(value)
			if err != nil {
				return err
			}
			opts.pids = append(opts.pids, pid)
		case maxUnchangedOpt:
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return fmt.Errorf("invalid number: '%s'", value)
			}
			opts.maxUnchanged = n
		default:
			// A digit can only be left when the obsolete form didn't
			// apply, like on "tail -n 2 -5".
			if isDigit(byte(opt)) {
				return fmt.Errorf("option used in invalid context -- '%c'", opt)
			}
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}

	switch {
	case opts.reverse && opts.follow:
		return options{}, nil, errors.New("the -f option is not valid with -r")
	case opts.reverse && opts.countSet && !opts.lines:
		return options{}, nil, errors.New("the -c option is not valid with -r")
	case opts.reverse && opts.fromStart:
		return options{}, nil, fmt.Errorf("invalid number: '+%d'", opts.count)
	}
	if !opts.countSet && !opts.reverse {
		opts.lines = true
		opts.count = 10
	}
	return opts, operands, nil
}

// parseSeconds parses the number of seconds to wait between iterations: a
// non-negative decimal number, with either a period or a comma as its
// decimal separator, as the GNU manual allows.
func parseSeconds(value string) (time.Duration, error) {
	s, err := strconv.ParseFloat(strings.Replace(value, ",", ".", 1), 64)
	if err != nil || math.IsNaN(s) || math.IsInf(s, 0) || s < 0 {
		return 0, fmt.Errorf("invalid number of seconds: '%s'", value)
	}
	if s > math.MaxInt64/float64(time.Second) {
		s = math.MaxInt64 / float64(time.Second)
	}
	return time.Duration(s * float64(time.Second)), nil
}

// parsePID parses the process ID given with --pid.
func parsePID(value string) (int, error) {
	pid, err := strconv.Atoi(value)
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid process id: '%s'", value)
	}
	return pid, nil
}

// parseCount parses the number option-argument of -c and -n: a decimal
// integer, optionally signed and optionally followed by a multiplicative
// suffix, as "1K" or "5KB". A '+' sign makes copying start relative to the
// beginning of the input and is required to be non-zero. A '-' sign counts
// from the end, like no sign at all.
func parseCount(value string) (count int, fromStart bool, err error) {
	n, _, err := sizeparse.ParseNumber(value)
	if err != nil || n > math.MaxInt {
		return 0, false, fmt.Errorf("invalid number: '%s'", value)
	}
	fromStart = strings.HasPrefix(value, "+")
	if fromStart && n == 0 {
		return 0, false, fmt.Errorf("invalid number: '%s'", value)
	}
	return int(n), fromStart, nil
}

// parseObsolete parses the obsolete first-operand form of the GNU manual,
// tail -[NUM][bcl][f] or tail +[NUM][bcl][f]: NUM counts lines, or 512-byte
// blocks with 'b', bytes with 'c', and a trailing 'f' has the meaning of -f.
// A leading '+' counts from the beginning of the input. The form is
// recognized only when it is the single option and at most one file operand
// follows it, and only when it doesn't conflict with the standard syntax. It
// returns the options it stands for with the arguments left after it, and
// ok=false when the obsolete form doesn't apply.
func parseObsolete(args []string) (options, []string, bool) {
	if !obsoleteShape(args) {
		return options{}, nil, false
	}

	arg := args[0]
	if len(arg) < 2 {
		return options{}, nil, false
	}
	rest := arg[1:]
	opts := options{lines: true}
	switch arg[0] {
	case '+':
		opts.fromStart = true
	case '-':
		// "-c" is missing its argument.
		if rest == "c" {
			return options{}, nil, false
		}
	default:
		return options{}, nil, false
	}

	// The count defaults to the default number of lines.
	n, i := 10, 0
	if i < len(rest) && isDigit(rest[i]) {
		n = 0
		for ; i < len(rest) && isDigit(rest[i]); i++ {
			if d := int(rest[i] - '0'); n <= (math.MaxInt-d)/10 {
				n = n*10 + d
			} else {
				n = math.MaxInt
			}
		}
	}
	switch {
	case i < len(rest) && rest[i] == 'b':
		if n > math.MaxInt/512 {
			n = math.MaxInt
		} else {
			n *= 512
		}
		opts.lines = false
		i++
	case i < len(rest) && (rest[i] == 'c' || rest[i] == 'l'):
		opts.lines = rest[i] == 'l'
		i++
	}
	if i < len(rest) && rest[i] == 'f' {
		opts.follow = true
		i++
	}
	if i < len(rest) {
		return options{}, nil, false
	}

	opts.count = n
	opts.countSet = true
	return opts, args[1:], true
}

// obsoleteShape reports whether args can hold the obsolete form: the
// obsolete option alone, or followed by a file operand, by "--", or by "--"
// and a file operand.
func obsoleteShape(args []string) bool {
	switch {
	case len(args) == 1:
		return true
	case len(args) == 2:
		return args[1] == "--" || !(len(args[1]) > 1 && args[1][0] == '-')
	case len(args) == 3:
		return args[1] == "--"
	}
	return false
}

func isDigit(b byte) bool {
	return '0' <= b && b <= '9'
}

// fileSize returns the current size of f.
func fileSize(f fsx.SeekFile) (int64, error) {
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// seekLastBytes seeks f to where the last count bytes begin.
func seekLastBytes(f fsx.SeekFile, count int) error {
	size, err := fileSize(f)
	if err != nil {
		return err
	}
	_, err = f.Seek(max(0, size-int64(count)), io.SeekStart)
	return err
}

// seekLastLines seeks f to where the last count lines begin.
//
// The file is scanned backwards in chunks, growing the region until count
// <newline> separators are found or the start of the file is reached. The
// scan starts before a trailing <newline>, which ends the last line rather
// than separating it.
func seekLastLines(f fsx.SeekFile, count int, delim byte) error {
	size, err := fileSize(f)
	if err != nil {
		return err
	}

	const chunk = 4096
	found := 0
	start := size
	for start > 0 {
		low := max(0, start-chunk)
		b := make([]byte, start-low)
		if _, err := f.Seek(low, io.SeekStart); err != nil {
			return err
		}
		if _, err := io.ReadFull(f, b); err != nil {
			return err
		}
		start = low

		i := len(b) - 1
		if start+int64(len(b)) == size && b[i] == delim {
			i--
		}
		for ; i >= 0; i-- {
			if b[i] != delim {
				continue
			}
			found++
			if found == count {
				_, err := f.Seek(start+int64(i)+1, io.SeekStart)
				return err
			}
		}
	}
	_, err = f.Seek(0, io.SeekStart)
	return err
}

// readLastBytes returns the last count bytes of r.
func readLastBytes(r io.Reader, count int) ([]byte, error) {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return nil, err
	}
	var buf []byte
	tmp := make([]byte, 32*1024)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if len(buf) > count {
			buf = buf[len(buf)-count:]
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return buf, nil
			}
			return nil, err
		}
	}
}

// readLastLines returns the last count lines of r, each including its
// trailing <newline> when the input has one.
func readLastLines(r io.Reader, count int, delim byte) ([][]byte, error) {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return nil, err
	}
	var lines [][]byte
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes(delim)
		if len(line) > 0 {
			lines = append(lines, line)
			if len(lines) > count {
				lines = lines[len(lines)-count:]
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return nil, err
		}
	}
}

// readAllLines returns all lines of r, each including its trailing <newline>
// when the input has one.
func readAllLines(r io.Reader, delim byte) ([][]byte, error) {
	var lines [][]byte
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes(delim)
		if len(line) > 0 {
			lines = append(lines, line)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return nil, err
		}
	}
}

// options holds the part of the input to copy.
type options struct {
	follow       bool // -f: keep copying as data becomes available
	byName       bool // --follow=name: track the name, not the descriptor
	retry        bool // --retry: keep trying to open the files
	reverse      bool // -r: write the lines in reverse order
	lines        bool // -n instead of -c
	countSet     bool // -c or -n given
	fromStart    bool // '+' sign: count from the beginning
	count        int
	headers      headerMode    // -q and -v: when the file headers are printed
	zero         bool          // -z: items end with NUL instead of <newline>
	pids         []int         // --pid: writers to watch
	sleep        time.Duration // -s: wait between iterations
	maxUnchanged int           // --max-unchanged-stats
	debug        bool          // --debug
}

// headerMode tells when the file headers are printed.
type headerMode int

const (
	headerAuto   headerMode = iota // only with more than one operand
	headerNever                    // -q: never
	headerAlways                   // -v: always
)

// delim returns the byte which ends an item of the input.
func (opts options) delim() byte {
	if opts.zero {
		return 0
	}
	return '\n'
}

// writeError marks an error as coming from writing to the standard output.
type writeError struct {
	err error
}

// Error implements the error interface.
func (e *writeError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make writeError unwrapable.
func (e *writeError) Unwrap() error {
	return e.err
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
