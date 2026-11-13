// Package fsop holds the operand plumbing shared by the utilities which
// copy, move, install or link files: the split of the operands into sources
// and a target, the source-and-destination pairs they work on, the symlink
// and time lookups they need, and the confirmations they ask the user.
package fsop

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/fsx"
)

// Pair is a source and a destination: the paths the file system knows them
// by, and the names diagnostics show.
type Pair struct {
	Src, Dst         string
	SrcName, DstName string
}

// Child returns the pair for an entry of the source directory.
func (e Pair) Child(name string) Pair {
	return Pair{
		Src:     path.Join(e.Src, name),
		Dst:     path.Join(e.Dst, name),
		SrcName: e.SrcName + "/" + name,
		DstName: e.DstName + "/" + name,
	}
}

// SplitOperands splits the operands into the sources and the target,
// following the -t and -T options.
func SplitOperands(operands []string, targetDir string, targetDirSet, noTarget bool) (sources []string, target string, err error) {
	if targetDirSet {
		if len(operands) == 0 {
			return nil, "", errors.New("missing file operand")
		}
		return operands, targetDir, nil
	}
	switch len(operands) {
	case 0:
		return nil, "", errors.New("missing file operand")
	case 1:
		return nil, "", fmt.Errorf("missing destination file operand after '%s'", operands[0])
	}
	if noTarget {
		if len(operands) > 2 {
			return nil, "", fmt.Errorf("extra operand '%s'", operands[2])
		}
		return operands[:1], operands[1], nil
	}
	return operands[:len(operands)-1], operands[len(operands)-1], nil
}

// ResolveSymlinks resolves every component of name, following the symbolic
// links found on the way, and returns the canonical path it names. Broken
// links and loops give up and keep the remaining components as they are.
func ResolveSymlinks(fsys fsx.FS, name string) string {
	var resolved []string
	for comp := range strings.SplitSeq(path.Clean(name), "/") {
		switch comp {
		case "", ".":
			continue
		case "..":
			if len(resolved) > 0 {
				resolved = resolved[:len(resolved)-1]
			}
			continue
		}
		resolved = append(resolved, comp)
		for range 40 {
			link := path.Join(resolved...)
			fi, err := fsys.Lstat(link)
			if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
				break
			}
			target, err := fsys.ReadLink(link)
			if err != nil {
				break
			}
			resolved = resolved[:len(resolved)-1]
			for tc := range strings.SplitSeq(path.Clean(target), "/") {
				switch tc {
				case "", ".":
				case "..":
					if len(resolved) > 0 {
						resolved = resolved[:len(resolved)-1]
					}
				default:
					resolved = append(resolved, tc)
				}
			}
		}
	}
	if len(resolved) == 0 {
		return "."
	}
	return path.Join(resolved...)
}

// Atime returns the access time of fi, falling back to its modification time.
func Atime(fi fs.FileInfo) time.Time {
	if t, ok := fsx.Atime(fi); ok {
		return t
	}
	return fi.ModTime()
}

// Confirm asks on the standard error, as "prog: action 'name'? ", whether the
// action should be taken on name, and reads the answer from the standard
// input. Only a response starting with 'y' or 'Y' is taken as affirmative.
func Confirm(rt *coreutils.Runtime, prog, action, name string) bool {
	fmt.Fprintf(rt.Stderr, "%s: %s '%s'? ", prog, action, name)
	if rt.Stdin == nil {
		return false
	}
	var (
		buf    [1]byte
		answer byte
	)
	for {
		n, err := rt.Stdin.Read(buf[:])
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if answer == 0 {
				answer = buf[0]
			}
		}
		if err != nil {
			break
		}
	}
	return answer == 'y' || answer == 'Y'
}
