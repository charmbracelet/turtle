package patch

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"charm.land/turtle/coreutils/internal/diff"
)

// runner holds the state of one patch run shared by every patch.
type runner struct {
	p            *Prog
	opts         options
	format       diff.PatchFormat
	backed       map[string]bool     // paths already holding a backup
	intermediate map[string][]string // target lines an earlier patch produced for an output file
	outWritten   bool                // whether the output file holds a version already
}

// patchOne applies one patch, returning whether any hunk failed or got
// ignored, which asks for an exit status of 1.
func (r *runner) patchOne(ctx context.Context, pr diff.Patch, operand string) (bool, error) {
	target, name, err := r.target(pr, operand)
	if err != nil {
		return false, err
	}
	lines, data, exists, err := r.readTarget(ctx, target)
	if err != nil {
		return false, &pathError{label: name, err: err}
	}
	if !exists && !creates(pr) {
		return false, errors.New("can't find file to patch: " + name)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	out, outcomes, skipped, reversed := r.apply(lines, pr)
	if skipped {
		// The patch was reversed or already applied, and skipped.
		return true, nil
	}
	if reversed {
		pr = diff.Reverse(pr)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if err := r.backup(ctx, target, data, exists); err != nil {
		return false, err
	}
	ignored, rejected := r.report(name, pr, outcomes)
	if err := r.writeResult(target, out, exists, pr); err != nil {
		return false, err
	}
	if err := r.writeRejects(pr, outcomes, target, name); err != nil {
		return false, err
	}
	return ignored || rejected, nil
}

// apply applies the hunks of the patch, handling one that comes out
// reversed or already applied: it gets skipped, or applied backwards when
// the answer to the prompt accepts it.
func (r *runner) apply(lines []string, pr diff.Patch) (out []string, outcomes []diff.Outcome, skipped, reversed bool) {
	applier := diff.Applier{Blank: r.opts.blank}
	out, outcomes = applier.Apply(lines, pr.Hunks)

	if r.opts.reverse || anyApplied(outcomes) {
		return out, outcomes, false, false
	}
	// No hunk applied: try the patch backwards, as one already applied.
	rev := diff.Reverse(pr)
	revOut, revOutcomes := applier.Apply(lines, rev.Hunks)
	if !anyApplied(revOutcomes) || anyFailed(revOutcomes) || anyAlready(revOutcomes) {
		return out, outcomes, false, false
	}
	if r.opts.ignore {
		r.p.messagef("Reversed (or previously applied) patch detected!  Skipping patch.\n")
		return nil, nil, true, false
	}
	if !yes(r.p.ask("Reversed (or previously applied) patch detected!  Assume -R? ")) {
		r.p.messagef("Skipping patch.\n")
		return nil, nil, true, false
	}
	return revOut, revOutcomes, false, true
}

// report writes the message of the patch and its hunks, returning whether
// any hunk got ignored or rejected.
func (r *runner) report(name string, pr diff.Patch, outcomes []diff.Outcome) (ignored, rejected bool) {
	r.p.messagef("patching file %s\n", name)
	failed := 0
	for i, o := range outcomes {
		at := pr.Hunks[i].FromLine
		switch {
		case o.Failed:
			failed++
			rejected = true
			r.p.messagef("Hunk #%d FAILED at %d.\n", i+1, at)
		case o.Already:
			ignored = true
			r.p.messagef("Hunk #%d ignored.\n", i+1)
		case o.Offset != 0 && o.Fuzz != 0:
			r.p.messagef("Hunk #%d succeeded at %d with fuzz %d (offset %d lines).\n", i+1, at+o.Offset, o.Fuzz, o.Offset)
		case o.Fuzz != 0:
			r.p.messagef("Hunk #%d succeeded at %d with fuzz %d.\n", i+1, at+o.Offset, o.Fuzz)
		case o.Offset != 0:
			r.p.messagef("Hunk #%d succeeded at %d (offset %d lines).\n", i+1, at+o.Offset, o.Offset)
		}
	}
	if failed > 0 {
		r.p.messagef("%d out of %d hunks FAILED -- saving rejects to file %s\n",
			failed, len(outcomes), r.rejectName(name))
	}
	return ignored, rejected
}

// writeResult writes the patched lines: to the output file of the -o
// option, versions appended, or to the file itself, which a patch that
// takes it away to /dev/null removes.
func (r *runner) writeResult(target string, out []string, exists bool, pr diff.Patch) error {
	if r.opts.output != "" {
		r.intermediate[target] = out
		outPath := r.p.resolvePath(r.opts.output, r.opts)
		if r.outWritten {
			return r.p.appendFile(outPath, out)
		}
		r.outWritten = true
		return r.p.writeFile(outPath, out, true)
	}
	if len(out) == 0 && pr.To == "/dev/null" {
		if exists {
			return r.p.FS.Remove(target)
		}
		return nil
	}
	return r.p.writeFile(target, out, exists)
}

// writeRejects writes the failed hunks of the patch to its reject file, in
// the format the patch came in.
func (r *runner) writeRejects(pr diff.Patch, outcomes []diff.Outcome, target, name string) error {
	var failed []int
	for i, o := range outcomes {
		if o.Failed {
			failed = append(failed, i)
		}
	}
	if len(failed) == 0 {
		return nil
	}
	f, err := r.p.FS.OpenFile(r.rejectPath(target), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^r.p.Umask.Perm())
	if err != nil {
		return err
	}
	defer f.Close()
	for _, i := range failed {
		if err := diff.WriteHunk(f, pr.Hunks[i], r.format); err != nil {
			return err
		}
	}
	return nil
}

// rejectPath returns the path of the reject file of a patched file.
func (r *runner) rejectPath(target string) string {
	if r.opts.reject != "" {
		return r.p.resolvePath(r.opts.reject, r.opts)
	}
	if r.opts.output != "" {
		return r.p.resolvePath(r.opts.output, r.opts) + ".rej"
	}
	return target + ".rej"
}

// rejectName returns the name of the reject file of a patched file, as the
// messages write it.
func (r *runner) rejectName(name string) string {
	if r.opts.reject != "" {
		return r.opts.reject
	}
	if r.opts.output != "" {
		return r.opts.output + ".rej"
	}
	return name + ".rej"
}

// backup saves the original contents before the first patch changes the
// file, in name.orig, or outfile.orig when writing to an output file.
func (r *runner) backup(ctx context.Context, target string, data []byte, exists bool) error {
	if !r.opts.backup {
		return nil
	}
	orig, content := target+".orig", data
	if r.opts.output != "" {
		out := r.p.resolvePath(r.opts.output, r.opts)
		orig = out + ".orig"
		if r.backed[orig] {
			return nil
		}
		if _, err := fs.Stat(r.p.FS, out); err != nil {
			return nil // nothing to save yet
		}
		var err error
		content, _, err = r.p.readFile(ctx, out)
		if err != nil {
			return err
		}
	} else if !exists || r.backed[orig] {
		return nil
	}
	if err := r.p.writeFileBytes(orig, content); err != nil {
		return err
	}
	r.backed[orig] = true
	return nil
}

// readTarget reads the lines of the file to patch, which an earlier patch
// of the same run may have already produced when writing to an output
// file.
func (r *runner) readTarget(ctx context.Context, target string) (lines []string, data []byte, exists bool, err error) {
	if r.opts.output != "" {
		if out, ok := r.intermediate[target]; ok {
			return out, nil, true, nil
		}
	}
	data, exists, err = r.p.readFile(ctx, target)
	if err != nil {
		return nil, nil, false, err
	}
	return diff.SplitLines(data), data, exists, nil
}

// target resolves the file the patch applies to: the operand when given,
// the header names otherwise, testing their existence, and finally asking
// for a name interactively. It returns the path and the name as the
// messages write it.
func (r *runner) target(pr diff.Patch, operand string) (path, name string, err error) {
	p := r.p
	if operand != "" {
		return p.resolvePath(operand, r.opts), operand, nil
	}
	for _, header := range []string{pr.From, pr.To, pr.Index} {
		if header == "" {
			continue
		}
		stripped := stripName(header, r.opts.strip)
		if stripped == "" {
			continue
		}
		path := p.resolvePath(stripped, r.opts)
		if r.exists(path) {
			return path, stripped, nil
		}
	}
	if creates(pr) && pr.To != "" {
		if stripped := stripName(pr.To, r.opts.strip); stripped != "" {
			return p.resolvePath(stripped, r.opts), stripped, nil
		}
	}
	name = p.ask("File to patch: ")
	if name == "" {
		return "", "", errors.New("can't find file to patch")
	}
	return p.resolvePath(name, r.opts), name, nil
}

// exists reports whether the path names a file.
func (r *runner) exists(path string) bool {
	_, err := fs.Stat(r.p.FS, path)
	return err == nil
}

// creates reports whether the patch adds a file.
func creates(pr diff.Patch) bool {
	if pr.From == "/dev/null" {
		return true
	}
	for _, h := range pr.Hunks {
		if len(h.Old) > 0 {
			return false
		}
	}
	return len(pr.Hunks) > 0
}

// readFile reads the whole file at the path, reporting whether it exists.
// Reads stop early when ctx is done.
func (p *Prog) readFile(ctx context.Context, path string) ([]byte, bool, error) {
	if p.FS == nil {
		return nil, false, errNoFS
	}
	f, err := p.FS.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()
	data, err := readAll(ctx, f)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// writeFile writes the lines to the path, creating the file when needed.
func (p *Prog) writeFile(path string, out []string, exists bool) error {
	perm := fs.FileMode(0o666 &^ p.Umask.Perm())
	if exists {
		if fi, err := fs.Stat(p.FS, path); err == nil {
			perm = fi.Mode().Perm()
		}
	}
	f, err := p.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, line := range out {
		if _, err := f.Write([]byte(line)); err != nil {
			return err
		}
	}
	return nil
}

// appendFile appends the lines to the output file, creating it when
// needed.
func (p *Prog) appendFile(path string, out []string) error {
	f, err := p.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666&^p.Umask.Perm())
	if err != nil {
		return err
	}
	defer f.Close()
	for _, line := range out {
		if _, err := f.Write([]byte(line)); err != nil {
			return err
		}
	}
	return nil
}

// writeFileBytes writes the data to the path, replacing it.
func (p *Prog) writeFileBytes(path string, data []byte) error {
	f, err := p.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^p.Umask.Perm())
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func anyApplied(outcomes []diff.Outcome) bool {
	for _, o := range outcomes {
		if !o.Failed && !o.Already {
			return true
		}
	}
	return false
}

func anyFailed(outcomes []diff.Outcome) bool {
	for _, o := range outcomes {
		if o.Failed {
			return true
		}
	}
	return false
}

func anyAlready(outcomes []diff.Outcome) bool {
	for _, o := range outcomes {
		if o.Already {
			return true
		}
	}
	return false
}
