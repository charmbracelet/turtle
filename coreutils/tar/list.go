package tar

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"charm.land/snail/coreutils/internal/fsop"
)

// list writes the names of the entries of the archive to the standard
// output, filtered by the operands when given. With -v, entries get a long
// format similar to ls -l.
func (a *archiver) list(ctx context.Context, operands []string) error {
	in, err := a.openReader()
	if err != nil {
		a.fail(a.archiveName(), err)
		return a.exit(1)
	}
	defer in.Close()
	tr, err := newTarReader(in)
	if err != nil {
		a.fail(a.archiveName(), err)
		return a.exit(1)
	}
	matched := matchTracker(operands)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			a.fail(a.archiveName(), errNotTar)
			break
		}
		if !nameMatches(operands, hdr.Name) {
			continue
		}
		markMatched(matched, hdr.Name)
		if a.opts.interactive && !fsop.Confirm(a.rt, "tar", "list", hdr.Name) {
			continue
		}
		if a.opts.verbose {
			fmt.Fprintln(a.rt.Stdout, longLine(hdr))
		} else {
			fmt.Fprintln(a.rt.Stdout, hdr.Name)
		}
	}
	a.reportMissing(operands, matched)
	return a.exit(1)
}

// longLine formats an entry like "ls -l" would: mode, owner, size,
// modification time and name, with the link target when there is one. The
// time is in UTC, so the listing doesn't depend on the machine's zone.
func longLine(hdr *tar.Header) string {
	line := fmt.Sprintf("%s %s %d %s %s",
		modeString(hdr.Typeflag, hdr.Mode),
		ownerName(hdr.Uid, hdr.Uname)+"/"+ownerName(hdr.Gid, hdr.Gname),
		hdr.Size,
		hdr.ModTime.UTC().Format("2006-01-02 15:04"),
		hdr.Name)
	switch hdr.Typeflag {
	case tar.TypeSymlink:
		line += " -> " + hdr.Linkname
	case tar.TypeLink:
		line += " link to " + hdr.Linkname
	}
	return line
}

// ownerName returns the name of an owner, or its numeric ID when there's no
// name.
func ownerName(id int, name string) string {
	if name != "" {
		return name
	}
	return strconv.Itoa(id)
}

// modeString formats the type and mode bits of an entry like ls does, as in
// "-rw-r--r--".
func modeString(typeflag byte, mode int64) string {
	b := []byte("----------")
	switch typeflag {
	case tar.TypeDir:
		b[0] = 'd'
	case tar.TypeSymlink:
		b[0] = 'l'
	case tar.TypeLink:
		b[0] = 'h'
	case tar.TypeChar:
		b[0] = 'c'
	case tar.TypeBlock:
		b[0] = 'b'
	case tar.TypeFifo:
		b[0] = 'p'
	case tar.TypeReg, tar.TypeRegA:
	default:
		b[0] = '?'
	}
	const chars = "rwxrwxrwx"
	for i := 1; i < len(b); i++ {
		if mode&(1<<uint(len(b)-1-i)) != 0 {
			b[i] = chars[i-1]
		}
	}
	if mode&0o4000 != 0 {
		b[3] = specialChar(b[3], 's')
	}
	if mode&0o2000 != 0 {
		b[6] = specialChar(b[6], 's')
	}
	if mode&0o1000 != 0 {
		b[9] = specialChar(b[9], 't')
	}
	return string(b)
}

// specialChar renders a setuid, setgid or sticky bit: lowercase when the
// execute bit below it is set, uppercase otherwise.
func specialChar(x byte, special byte) byte {
	if x == 'x' {
		return special
	}
	return special - 'a' + 'A'
}
