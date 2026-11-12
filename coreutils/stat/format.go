package stat

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils/internal/filemode"
	"charm.land/snail/coreutils/internal/printfmt"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/strftime"
	"charm.land/snail/coreutils/internal/usernames"
)

// The formats of a run: the default ones of the files and the file systems,
// and the terse ones, which the manual identifies with an explicit --format.
const (
	defaultFormat = "  File: %N\n" +
		"  Size: %-10s\tBlocks: %-10b IO Block: %-6o %F\n" +
		"Device: %Hd,%Ld\tInode: %-10i  Links: %h\n" +
		"Access: (%04a/%10.10A)  Uid: (%5u/%8U)   Gid: (%5g/%8G)\n" +
		"Access: %x\n" +
		"Modify: %y\n" +
		"Change: %z\n" +
		" Birth: %w\n"

	defaultFormatDevice = "  File: %N\n" +
		"  Size: %-10s\tBlocks: %-10b IO Block: %-6o %F\n" +
		"Device: %Hd,%Ld\tInode: %-10i  Links: %-5h Device type: %Hr,%Lr\n" +
		"Access: (%04a/%10.10A)  Uid: (%5u/%8U)   Gid: (%5g/%8G)\n" +
		"Access: %x\n" +
		"Modify: %y\n" +
		"Change: %z\n" +
		" Birth: %w\n"

	fsFormat = "  File: %Qn\n" +
		"    ID: %-8i Namelen: %-7l Type: %T\n" +
		"Block size: %-10s Fundamental block size: %S\n" +
		"Blocks: Total: %-10b Free: %-10f Available: %a\n" +
		"Inodes: Total: %-10c Free: %d\n"

	terseFormat = "%Qn %s %b %f %u %g %D %i %h %t %T %X %Y %Z %W %o\n"

	terseFSFormat = "%Qn %i %l %t %s %S %b %f %a %c %d\n"
)

// renderer writes the fields of a file in a format, holding the state of a
// run: the zone timestamps are written in, the quoting style of the names,
// and the names of users and groups it looked up.
type renderer struct {
	prog  *Prog
	loc   *time.Location
	style quotingStyle
	names usernames.Names
}

// newRenderer builds the renderer of a run, resolving the quoting style the
// QUOTING_STYLE variable names, which defaults to shell-escape.
func (p *Prog) newRenderer() *renderer {
	r := &renderer{
		prog:  p,
		loc:   p.location(),
		style: quoteShellEscape,
		names: usernames.Names{Unknown: "UNKNOWN"},
	}
	if word, ok := p.LookupEnv("QUOTING_STYLE"); ok {
		if style, found := quotingStyles[word]; found {
			r.style = style
		} else {
			stdopts.Errorf(p.Runtime, "stat", "ignoring invalid value of environment variable QUOTING_STYLE: %s", word)
		}
	}
	return r
}

// render writes the format for a file, with its backslash escapes taken
// when escapes asks for it and the file system conversions when fsMode
// does. It reports whether any directive failed for the file, and a fatal
// error when the format holds an invalid directive.
func (r *renderer) render(format string, st *fileStat, fsMode, escapes bool) (out string, fail bool, err error) {
	if escapes {
		if format, err = r.expand(format); err != nil {
			return "", false, err
		}
	}
	var b strings.Builder
	for i := 0; i < len(format); {
		if format[i] != '%' {
			b.WriteByte(format[i])
			i++
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			b.WriteByte('%')
			i += 2
			continue
		}
		start := i
		spec, off := printfmt.ScanSpec(format[i:], false)
		if off == len(format[i:]) {
			// The format ends within the directive: a bare '%' is written
			// as it is, and one carrying flags or a width is invalid. The
			// text before it is kept, the way GNU leaves what it wrote.
			if spec.Raw != "" {
				return b.String(), false, fmt.Errorf("'%s': invalid directive", format[i:i+off])
			}
			b.WriteByte('%')
			break
		}
		c, mod := format[i+off], byte(0)
		quoted := false
		switch {
		case (c == 'H' || c == 'L') && i+off+1 < len(format) &&
			(format[i+off+1] == 'd' || format[i+off+1] == 'r'):
			mod, c, i = c, format[i+off+1], i+off+2
		case c == 'Q' && i+off+1 < len(format) && format[i+off+1] == 'n':
			c, quoted, i = 'n', true, i+off+2
		default:
			i += off + 1
		}
		if c == '%' {
			// A '%' conversion carrying flags, like "%5%", is invalid.
			// The text before it is kept, the way GNU leaves what it
			// wrote before the directive fails it.
			return b.String(), false, fmt.Errorf("'%s': invalid directive", format[start:i])
		}
		text, dfail, known := r.directive(c, quoted, mod, spec, st, fsMode)
		if !known {
			b.WriteByte('?')
			continue
		}
		fail = fail || dfail
		b.WriteString(text)
	}
	return b.String(), fail, nil
}

// expand takes the backslash escapes of a --printf format, warning about
// the ones it doesn't take the way GNU does.
func (r *renderer) expand(format string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '\\' {
			b.WriteByte(format[i])
			continue
		}
		if i+1 == len(format) {
			stdopts.Errorf(r.prog.Runtime, "stat", "warning: backslash at end of format")
			b.WriteByte('\\')
			break
		}
		esc, n, err := printfmt.ScanEscape(format[i+1:], printfmt.Escapes{
			Octal:    printfmt.OctalZero,
			Extended: true,
		})
		if err != nil {
			return "", err
		}
		if esc.Stop || esc.Unknown {
			stdopts.Errorf(r.prog.Runtime, "stat", "warning: unrecognized escape '\\%c'", format[i+1])
			b.WriteByte(format[i+1])
		} else {
			b.WriteString(esc.Lit)
		}
		i += n
	}
	return b.String(), nil
}

// directive writes one conversion of a file, as the conversions of the
// files and the file systems are two different sets. It reports whether the
// conversion failed for the file, and whether it is a known one at all.
func (r *renderer) directive(c byte, quoted bool, mod byte, spec printfmt.Spec, st *fileStat, fsMode bool) (string, bool, bool) {
	if fsMode {
		return r.fsDirective(c, quoted, spec, st)
	}
	return r.fileDirective(c, quoted, mod, spec, st)
}

// fileDirective writes one conversion about a file.
func (r *renderer) fileDirective(c byte, quoted bool, mod byte, spec printfmt.Spec, st *fileStat) (string, bool, bool) {
	m := st.fi.Mode()
	switch c {
	case 'a':
		return numField(uint64(filemode.Octal(m)), spec, 8), false, true
	case 'A':
		return strField(filemode.String(m), spec), false, true
	case 'b':
		return numField(st.blocks, spec, 10), false, true
	case 'B':
		return numField(512, spec, 10), false, true
	case 'C':
		stdopts.Errorf(r.prog.Runtime, "stat", "failed to get security context of '%s'", st.name)
		return "?", true, true
	case 'd':
		return numField(devicePart(st.dev, mod), spec, 10), false, true
	case 'D':
		return numField(st.dev, spec, 16), false, true
	case 'f':
		return numField(rawMode(m), spec, 16), false, true
	case 'F':
		return strField(fileType(m), spec), false, true
	case 'g':
		return numField(uint64(st.gid), spec, 10), false, true
	case 'G':
		return strField(r.names.Group(uint64(st.gid)), spec), false, true
	case 'h':
		return numField(st.nlink, spec, 10), false, true
	case 'i':
		return numField(st.ino, spec, 10), false, true
	case 'm':
		return "?", true, true
	case 'n':
		name := st.name
		if quoted {
			name = quoteName(name, r.style)
		}
		return strField(name, spec), false, true
	case 'N':
		name := quoteName(st.name, r.style)
		switch {
		case !st.isLink || st.follows:
			return strField(name, spec), false, true
		case st.linkErr != nil:
			stdopts.Errorf(r.prog.Runtime, "stat", "cannot read symbolic link '%s': %v", st.name, st.linkErr)
			return "", true, true
		default:
			name += " -> " + quoteName(st.target, r.style)
			return strField(name, spec), false, true
		}
	case 'o':
		return numField(uint64(st.blksize), spec, 10), false, true
	case 'r':
		return numField(devicePart(st.rdev, mod), spec, 10), false, true
	case 'R':
		return numField(st.rdev, spec, 16), false, true
	case 's':
		return numField(uint64(st.fi.Size()), spec, 10), false, true
	case 't':
		return numField(major(st.rdev), spec, 16), false, true
	case 'T':
		return numField(minor(st.rdev), spec, 16), false, true
	case 'u':
		return numField(uint64(st.uid), spec, 10), false, true
	case 'U':
		return strField(r.names.User(uint64(st.uid)), spec), false, true
	case 'w':
		if !st.hasBtime {
			return strField("-", spec), false, true
		}
		return r.humanTime(st.btime, spec), false, true
	case 'W':
		if !st.hasBtime {
			return numField(0, spec, 10), false, true
		}
		return r.epochField(st.btime, spec), false, true
	case 'x':
		return r.humanTime(st.atime, spec), false, true
	case 'X':
		return r.epochField(st.atime, spec), false, true
	case 'y':
		return r.humanTime(st.mtime, spec), false, true
	case 'Y':
		return r.epochField(st.mtime, spec), false, true
	case 'z':
		return r.humanTime(st.ctime, spec), false, true
	case 'Z':
		return r.epochField(st.ctime, spec), false, true
	default:
		return "", false, false
	}
}

// fsDirective writes one conversion about the file system a file is on.
// The file system abstraction reports none of the statistics here, so every
// field but the name is written as a question mark, the way GNU writes the
// values a platform doesn't report.
func (r *renderer) fsDirective(c byte, quoted bool, spec printfmt.Spec, st *fileStat) (string, bool, bool) {
	switch c {
	case 'n':
		name := st.name
		if quoted {
			name = quoteName(name, r.style)
		}
		return strField(name, spec), false, true
	default:
		return "?", false, true
	}
}

// numField writes an unsigned number within the width the spec asks for,
// with the octal and hexadecimal ones prefixed by the # flag.
func numField(v uint64, spec printfmt.Spec, base int) string {
	s := strconv.FormatUint(v, base)
	if strings.ContainsRune(spec.Flags, '#') {
		switch base {
		case 8:
			if len(s) == 0 || s[0] != '0' {
				s = "0" + s
			}
		case 16:
			if s != "0" {
				s = "0x" + s
			}
		}
	}
	return padField(s, spec, true)
}

// strField writes a string within the width the spec asks for, cut to its
// precision when it has one.
func strField(s string, spec printfmt.Spec) string {
	if spec.Prec >= 0 && spec.Prec < int64(len(s)) {
		s = s[:spec.Prec]
	}
	return padField(s, spec, false)
}

// padField pads s to the width of the spec, with zeros after a leading
// sign when the field is numeric and the 0 flag asks for it.
func padField(s string, spec printfmt.Spec, numeric bool) string {
	w := int(spec.Width)
	if w < 0 || len(s) >= w {
		return s
	}
	fill := " "
	if numeric && !strings.ContainsRune(spec.Flags, '-') && strings.ContainsRune(spec.Flags, '0') {
		fill = "0"
	}
	if strings.ContainsRune(spec.Flags, '-') {
		return s + strings.Repeat(fill, w-len(s))
	}
	if fill == "0" && strings.HasPrefix(s, "-") {
		return "-" + strings.Repeat(fill, w-len(s)) + s[1:]
	}
	return strings.Repeat(fill, w-len(s)) + s
}

// epochField writes a time as seconds since the Epoch, with the fraction of
// the precision the spec asks for, or none when it has no period at all.
// The excess precision of a fraction is discarded toward minus infinity,
// so a time before the Epoch is floored at the digits it keeps.
func (r *renderer) epochField(t time.Time, spec printfmt.Spec) string {
	prec := -1
	if spec.Prec >= 0 {
		prec = 9
		if !strings.HasSuffix(spec.Raw, ".") {
			prec = int(min(spec.Prec, math.MaxInt32))
		}
	}
	sec, nsec := t.Unix(), uint64(t.Nanosecond())
	if prec <= 0 {
		return padField(strconv.FormatInt(sec, 10), spec, true)
	}
	// The digits written are the ones of the time floored at prec digits:
	// the seconds scaled by 10^prec, plus the fraction of the second, with
	// the digits beyond the nanoseconds a time holds as zeros.
	var s string
	switch {
	case sec >= 0:
		s = strconv.FormatInt(sec, 10) + "." + fractionDigits(nsec, prec)
	case fractionIsZero(nsec, prec):
		s = strconv.FormatInt(sec, 10) + "." + strings.Repeat("0", prec)
	default:
		s = "-" + strconv.FormatUint(uint64(-sec-1), 10) + "." + deficitDigits(nsec, prec)
	}
	return padField(s, spec, true)
}

// fractionDigits returns the prec fraction digits of a positive fraction of
// a second, floored: the nanoseconds cut to the digits kept, followed by
// zeros when the precision reaches beyond them.
func fractionDigits(nsec uint64, prec int) string {
	if prec <= 9 {
		return fracString(nsec/pow10(9-prec), prec)
	}
	return fracString(nsec, 9) + strings.Repeat("0", prec-9)
}

// deficitDigits returns the prec fraction digits of the distance of a
// fraction of a second from the second above it, which the negative values
// floor with: the deficit ceiled to the digits kept, followed by zeros when
// the precision reaches beyond the nanoseconds.
func deficitDigits(nsec uint64, prec int) string {
	if prec <= 9 {
		return fracString(pow10(prec)-nsec/pow10(9-prec), prec)
	}
	return fracString(uint64(time.Second)-nsec, 9) + strings.Repeat("0", prec-9)
}

// fractionIsZero reports whether a fraction of a second vanishes at prec
// digits, so a negative time floors to a whole second.
func fractionIsZero(nsec uint64, prec int) bool {
	if prec <= 9 {
		return nsec < pow10(9-prec)
	}
	return nsec == 0
}

// fracString writes a fraction held in units of 10^-prec seconds to prec
// digits.
func fracString(v uint64, prec int) string {
	s := strconv.FormatUint(v, 10)
	return strings.Repeat("0", prec-len(s)) + s
}

// pow10 returns ten to the given power, which the callers keep within the
// range a uint64 holds.
func pow10(n int) uint64 {
	v := uint64(1)
	for range n {
		v *= 10
	}
	return v
}

// humanTime writes a time as GNU writes the %x, %y and %z conversions: the
// full ISO form, with the nine digits of its fraction cut to the precision
// the spec asks for.
func (r *renderer) humanTime(t time.Time, spec printfmt.Spec) string {
	t = t.In(r.loc)
	prec := 9
	if spec.Prec >= 0 {
		prec = int(min(spec.Prec, math.MaxInt32))
		if strings.HasSuffix(spec.Raw, ".") {
			prec = 9
		}
	}
	s := strftime.Format("%Y-%m-%d %H:%M:%S", t)
	if prec > 0 {
		s += "." + fracDigits(uint64(t.Nanosecond()), prec)
	}
	s += " " + strftime.Format("%z", t)
	spec.Prec = -1
	return strField(s, spec)
}

// fracDigits writes the fraction of a time to the given number of digits,
// cutting the nanoseconds toward minus infinity.
func fracDigits(nsec uint64, prec int) string {
	digits := fmt.Sprintf("%09d", nsec)
	if prec <= 9 {
		return digits[:prec]
	}
	return digits + strings.Repeat("0", prec-9)
}

// location returns the zone timestamps are written in, honoring TZ. The
// zone names, like "America/New_York", are looked up in the system
// database, and anything else falls back to the local zone.
func (p *Prog) location() *time.Location {
	if tz, ok := p.LookupEnv("TZ"); ok && tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}
