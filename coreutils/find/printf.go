package find

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils/internal/fileinfo"
	"charm.land/snail/coreutils/internal/filemode"
	"charm.land/snail/coreutils/internal/printfmt"
	"charm.land/snail/coreutils/internal/sizefmt"
	"charm.land/snail/coreutils/internal/strftime"

	"charm.land/snail/fsx"
)

// ctimeFormat is the date and time format the bare %a, %c and %t directives
// write, like the C ctime function.
const ctimeFormat = "%a %b %e %H:%M:%S %Y"

// printfFormat is a compiled -printf or -fprintf format.
type printfFormat struct {
	pieces []printfPiece
}

// printfPiece is one piece of a -printf format: literal text, an escaped
// character, or a directive.
type printfPiece struct {
	lit     string // literal text, or the text an escape stands for
	dir     byte   // the directive character, 0 for literal text
	spec    string // the printf style flags, width and precision
	tformat string // the format of a time directive, for %A, %C and %T
	stop    bool   // \c: stop printing and flush
}

var printfEscapes = printfmt.Escapes{Octal: printfmt.OctalPlain}

// compilePrintf compiles a -printf format, reporting the escapes and
// directives it doesn't know on the warning writer, which may be nil.
func compilePrintf(warnings io.Writer, format string) (printfFormat, error) {
	f := printfFormat{}
	warn := func(format string, args ...any) {
		if warnings == nil {
			return
		}
		fmt.Fprintf(warnings, "find: warning: "+format+"\n", args...)
	}
	for i := 0; i < len(format); {
		switch c := format[i]; c {
		case '\\':
			i++
			if i >= len(format) {
				f.pieces = append(f.pieces, printfPiece{lit: "\\"})
				break
			}
			esc, size, err := printfmt.ScanEscape(format[i:], printfEscapes)
			if err != nil {
				return printfFormat{}, err
			}
			if esc.Unknown {
				// A backslash followed by any other character is treated as
				// an ordinary one, so both are printed.
				warn("unrecognised escape '\\%c'", format[i])
			}
			if esc.Stop {
				f.pieces = append(f.pieces, printfPiece{stop: true})
			} else {
				f.pieces = append(f.pieces, printfPiece{lit: esc.Lit})
			}
			i += size
		case '%':
			piece, next, err := parseDirective(format, i, warn)
			if err != nil {
				return printfFormat{}, err
			}
			f.pieces = append(f.pieces, piece)
			i = next
		default:
			j := strings.IndexAny(format[i:], "\\%")
			if j < 0 {
				j = len(format) - i
			}
			f.pieces = append(f.pieces, printfPiece{lit: format[i : i+j]})
			i += j
		}
	}
	return f, nil
}

// parseDirective parses one % directive, starting at the % of format[i],
// and returns where the next piece starts.
func parseDirective(format string, i int, warn func(string, ...any)) (printfPiece, int, error) {
	spec, j := printfmt.ScanSpec(format[i:], false)
	if i+j >= len(format) {
		// A '%' at the end of the format: its behavior is undefined, so
		// it's taken literally.
		return printfPiece{lit: "%"}, i + j, nil
	}
	dir := format[i+j]
	j++
	switch dir {
	case '%':
		return printfPiece{lit: "%"}, i + j, nil
	case 'p', 'f', 'h', 'P', 'H', 'g', 'G', 'u', 'U', 'm', 'M',
		'n', 's', 'b', 'k', 'S', 'd', 'D', 'i', 'l', 'y', 'Y',
		'a', 'c', 't':
		return printfPiece{dir: dir, spec: spec.Raw}, i + j, nil
	case 'A', 'B', 'C', 'T':
		piece := printfPiece{dir: dir, spec: spec.Raw}
		if i+j < len(format) {
			piece.tformat = string(format[i+j])
			j++
		}
		return piece, i + j, nil
	case '(', '{', '[':
		return printfPiece{}, 0, fmt.Errorf("the '%%%c' directive is reserved for future use", dir)
	default:
		// The directive is discarded, but the character is printed.
		warn("unrecognised directive '%%%c'", dir)
		return printfPiece{lit: string(dir)}, i + j, nil
	}
}

// write writes the format for one file.
func (f printfFormat) write(s *state, w io.Writer, file file) error {
	for _, piece := range f.pieces {
		if piece.stop {
			// \c: stop printing from this format and flush the output.
			if flusher, ok := w.(interface{ Flush() error }); ok {
				return flusher.Flush()
			}
			return nil
		}
		if piece.dir == 0 {
			if _, err := io.WriteString(w, piece.lit); err != nil {
				return err
			}
			continue
		}
		if _, err := io.WriteString(w, f.directive(s, piece, file)); err != nil {
			return err
		}
	}
	return nil
}

// directive returns what one % directive writes for a file.
func (f printfFormat) directive(s *state, piece printfPiece, file file) string {
	if piece.dir == 'd' {
		// Only %d supports the numeric flags, like the GNU one.
		return fmt.Sprintf("%"+piece.spec+"d", file.depth)
	}
	return piece.text(s, file)
}

// writeMode returns what %m writes, honoring the '#' flag, which makes it
// keep a leading zero.
func (piece printfPiece) writeMode(m fs.FileMode) string {
	value := strconv.FormatUint(uint64(filemode.Octal(m)), 8)
	if strings.Contains(piece.spec, "#") && value != "0" {
		value = "0" + value
	}
	return fmt.Sprintf("%"+cleanSpec(piece.spec)+"s", value)
}

// text returns the string one directive writes.
func (piece printfPiece) text(s *state, file file) string {
	spec := cleanSpec(piece.spec)
	format := func(value string) string {
		return fmt.Sprintf("%"+spec+"s", value)
	}
	st, stOK := fileinfo.Stat(file.fi)
	uid, gid, _ := fsx.Owner(file.fi)
	switch piece.dir {
	case 'p':
		return format(file.name)
	case 'f':
		return format(basename(file.name))
	case 'h':
		return format(dirname(file.name))
	case 'P':
		return format(file.relName())
	case 'H':
		return format(file.root)
	case 'g':
		return format(s.usernames.Group(uint64(gid)))
	case 'G':
		return format(strconv.Itoa(gid))
	case 'u':
		return format(s.usernames.User(uint64(uid)))
	case 'U':
		return format(strconv.Itoa(uid))
	case 'm':
		return piece.writeMode(file.fi.Mode())
	case 'M':
		return format(filemode.String(file.fi.Mode()))
	case 'n':
		nlink := 0
		if stOK {
			nlink = int(st.Nlink)
		}
		return format(strconv.Itoa(nlink))
	case 's':
		return format(strconv.FormatInt(file.fi.Size(), 10))
	case 'b':
		blocks := (file.fi.Size() + 511) / 512
		if stOK {
			blocks = int64(st.Blocks)
		}
		return format(strconv.FormatInt(blocks, 10))
	case 'k':
		blocks := (file.fi.Size() + 1023) / 1024
		if stOK {
			blocks = int64((st.Blocks*512 + 1023) / 1024)
		}
		return format(strconv.FormatInt(blocks, 10))
	case 'S':
		size := file.fi.Size()
		sparse := 1.0
		if stOK && size > 0 {
			sparse = float64(st.Blocks*512) / float64(size)
		}
		return format(fmt.Sprintf("%.1f", sparse))
	case 'D':
		dev := uint64(0)
		if stOK {
			dev = st.Dev
		}
		return format(strconv.FormatUint(dev, 10))
	case 'i':
		ino := uint64(0)
		if stOK {
			ino = st.Ino
		}
		return format(strconv.FormatUint(ino, 10))
	case 'l':
		return format(s.linkTarget(file))
	case 'y':
		return format(string(typeLetter(file.fi.Mode())))
	case 'Y':
		return format(string(targetType(s, file)))
	case 'a', 'c', 't', 'A', 'C', 'T', 'B':
		return format(timeString(s, file, piece))
	}
	return ""
}

// timeString returns what a time directive writes.
func timeString(s *state, file file, piece printfPiece) string {
	switch piece.dir {
	case 'a':
		return strftime.Format(ctimeFormat, s.timeOf(file.fi, 'a'))
	case 'c':
		return strftime.Format(ctimeFormat, s.timeOf(file.fi, 'c'))
	case 't':
		return strftime.Format(ctimeFormat, file.fi.ModTime())
	case 'B':
		// No file system here reports a birth time.
		return ""
	}
	t := file.fi.ModTime()
	switch piece.dir {
	case 'A':
		t = s.timeOf(file.fi, 'a')
	case 'C':
		t = s.timeOf(file.fi, 'c')
	}
	return timeComponent(piece.tformat, t)
}

// timeComponent returns what one %Ak, %Ck or %Tk format writes.
func timeComponent(k string, t time.Time) string {
	switch k {
	case "":
		return strftime.Format(ctimeFormat, t)
	case "@":
		// The seconds since the epoch, with a fractional part.
		return fmt.Sprintf("%d.%09d", t.Unix(), t.Nanosecond())
	case "S":
		return fmt.Sprintf("%02d.%09d", t.Second(), t.Nanosecond())
	case "T":
		return fmt.Sprintf("%02d:%02d:%02d.%09d", t.Hour(), t.Minute(), t.Second(), t.Nanosecond())
	case "X":
		return fmt.Sprintf("%02d:%02d:%02d.%09d", t.Hour(), t.Minute(), t.Second(), t.Nanosecond())
	case "+":
		return fmt.Sprintf("%04d-%02d-%02d+%02d:%02d:%02d.%09d",
			t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond())
	}
	return strftime.Format("%"+k, t)
}

// targetType returns the type letter %Y writes: the one of the target of a
// symbolic link, N when it's broken, L when it loops, and ? for any other
// failure.
func targetType(s *state, file file) byte {
	if file.fi.Mode()&fs.ModeSymlink == 0 {
		return typeLetter(file.fi.Mode())
	}
	fi, err := fs.Stat(s.p.FS, file.fsName)
	if err != nil {
		switch {
		case isNotExist(err):
			return 'N'
		case isLoop(err):
			return 'L'
		default:
			return '?'
		}
	}
	return typeLetter(fi.Mode())
}

// typeLetter returns the type letter the -type primary and the %y and %Y
// directives write.
func typeLetter(m fs.FileMode) byte {
	switch filemode.TypeChar(m) {
	case 'd', 'l', 'p', 's', 'c', 'b':
		return filemode.TypeChar(m)
	case '?':
		return '?'
	}
	return 'f'
}

// isNotExist reports whether an error says a file doesn't exist.
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// isLoop reports whether an error says a symbolic link loops. The error
// reports no such dedicated kind, so its message is matched.
func isLoop(err error) bool {
	if perr, ok := errors.AsType[*fs.PathError](err); ok {
		return strings.Contains(perr.Err.Error(), "too many levels")
	}
	return false
}

// cleanSpec returns the part of a printf spec the string directives take:
// the '-' flag, the width and the precision.
func cleanSpec(spec string) string {
	var out strings.Builder
	dot := false
	for _, c := range spec {
		switch {
		case c == '-':
			out.WriteRune(c)
		case c == '.':
			if dot {
				continue
			}
			dot = true
			out.WriteRune(c)
		case c >= '0' && c <= '9':
			out.WriteRune(c)
		}
	}
	return out.String()
}

// parsePrintf parses the -printf and -fprintf primaries.
func (p *parser) parsePrintf(name string) (expr, error) {
	var file string
	if name == "-fprintf" {
		out, err := p.arg(name)
		if err != nil {
			return nil, err
		}
		file = out
	}
	format, err := p.arg(name)
	if err != nil {
		return nil, err
	}
	compiled, err := compilePrintf(p.rt.Stderr, format)
	if err != nil {
		return nil, fmt.Errorf("%v for %s", err, name)
	}
	return printfExpr{format: compiled, file: file}, nil
}

// listFile writes one line of the -ls and -fls actions, in the layout of
// ls -dils.
func listFile(s *state, w io.Writer, f file) error {
	st, _ := fileinfo.Stat(f.fi)
	blocks := (f.fi.Size() + 1023) / 1024
	if st.Dev != 0 || st.Ino != 0 {
		blocks = int64((st.Blocks*512 + 1023) / 1024)
	}
	uid, gid, _ := fsx.Owner(f.fi)
	nlink := 1
	if st.Nlink != 0 {
		nlink = int(st.Nlink)
	}
	date := fileTime(f.fi, 'm')
	_, err := fmt.Fprintf(w, "%7d %6d %s %3d %-8s %-8s %8s %s %s",
		st.Ino, blocks, filemode.String(f.fi.Mode()), nlink,
		s.usernames.User(uint64(uid)), s.usernames.Group(uint64(gid)),
		sizefmt.ByteSpec.Format(f.fi.Size()), lsTime(s, date), quoteName(f.name))
	if err != nil {
		return err
	}
	if f.fi.Mode()&fs.ModeSymlink != 0 {
		if target := s.linkTarget(f); target != "" {
			if _, err := fmt.Fprintf(w, " -> %s", quoteName(target)); err != nil {
				return err
			}
		}
	}
	_, err = fmt.Fprintln(w)
	return err
}

// lsTime returns the date a -ls line writes: the time of day for the files
// of the last six months, and the year for the older ones.
func lsTime(s *state, t time.Time) string {
	cutoff := s.now.Add(-182 * 24 * time.Hour)
	if t.After(cutoff) && t.Before(s.now.Add(24*time.Hour)) {
		return strftime.Format("%b %e %H:%M", t)
	}
	return strftime.Format("%b %e  %Y", t)
}

// quoteName writes a name the way -ls does, escaping the characters that
// aren't printable with backslash escapes.
func quoteName(name string) string {
	quoted := false
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 0x20 && c < 0x7f || c == '\t' || c == '\n' || c == '\r' {
			switch c {
			case '\\', '"':
				quoted = true
				b.WriteByte('\\')
			}
			b.WriteByte(c)
			continue
		}
		quoted = true
		switch c {
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\v':
			b.WriteString(`\v`)
		default:
			b.WriteString(`\`)
			b.WriteString(fmt.Sprintf("%03o", c))
		}
	}
	if quoted {
		return `"` + b.String() + `"`
	}
	return b.String()
}
