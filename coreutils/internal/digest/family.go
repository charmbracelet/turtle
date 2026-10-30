package digest

import (
	"context"
	"errors"
	"fmt"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Family is a checksum utility, like shasum, which selects the algorithm of
// each run with an -a option instead of having one fixed algorithm. It
// follows the Perl shasum script of the Digest::SHA distribution: SHA-1
// unless -a says otherwise, and no --zero.
type Family struct {
	// Name is the program name, used on diagnostic messages.
	Name string

	// Algorithms maps the values of the -a option to the algorithms they
	// select. The Name of each one is the name of the family.
	Algorithms map[string]Algorithm

	// Default is the value of the -a option used when it isn't given.
	Default string
}

// Run writes the digest of each operand to the standard output, or verifies
// the checksum lines read from them when -c is given, hashing with the
// algorithm the -a option selects. The standard input is used when no
// operand is given, and also when an operand is "-".
//
// It returns a [*coreutils.ExitError] when an operand failed, and the context
// error when ctx is done.
func (f Family) Run(ctx context.Context, rt *coreutils.Runtime, args []string) error {
	if done, err := stdopts.Check(rt, f.Name, usage(f.Name), familyOptSet, args); done {
		return err
	}
	set, err := f.parseFlags(args)
	if err != nil {
		f.errorf(rt, "%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if err := set.validate(); err != nil {
		f.errorf(rt, "%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	a, ok := f.Algorithms[set.algorithm]
	if !ok {
		f.errorf(rt, "unrecognized algorithm '%s'", set.algorithm)
		return &coreutils.ExitError{Code: 1}
	}
	a.Name = f.Name
	operands := set.operands
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	if set.check {
		return a.verify(ctx, rt, set.options, operands)
	}
	return a.digests(ctx, rt, set.options, operands)
}

// errorf writes a diagnostic message to the standard error of rt.
func (f Family) errorf(rt *coreutils.Runtime, format string, args ...any) {
	fmt.Fprintf(rt.Stderr, f.Name+": "+format+"\n", args...)
}

// settings holds what a run of a family utility selects: the options shared
// with the {prog}sum utilities and the algorithm its -a option selects.
type settings struct {
	options

	algorithm string
	operands  []string

	// binaryGiven and textGiven tell the modes the -b and -t options asked
	// for, which the Perl script refuses to take together.
	binaryGiven bool
	textGiven   bool
}

// familyLongOptions maps the long options of the family utilities to the
// short ones they stand for.
var familyLongOptions = map[string]rune{
	"algorithm":      'a',
	"binary":         'b',
	"check":          'c',
	"ignore-missing": optIgnoreMissing,
	"quiet":          optQuiet,
	"status":         optStatus,
	"strict":         optStrict,
	"tag":            optTag,
	"text":           't',
	"warn":           'w',
}

// familyOptSet describes the options of the family utilities.
var familyOptSet = getopt.Set{Valued: "a", Long: familyLongOptions}

// parseFlags parses the options and operands, along with the algorithm the
// -a option selects. Option parsing stops at the first operand or at "--".
func (f Family) parseFlags(args []string) (settings, error) {
	set := settings{algorithm: f.Default}
	operands, err := familyOptSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			set.algorithm = value
		case 'b':
			set.binary, set.binaryGiven = true, true
		case 'c':
			set.check = true
		case 't':
			set.binary, set.textGiven = false, true
		case 'w':
			set.status, set.quiet = false, false
			set.warn = true
		case 'q', optQuiet:
			set.status, set.warn = false, false
			set.quiet = true
		case 's', optStatus:
			set.warn, set.quiet = false, false
			set.status = true
		case optIgnoreMissing:
			set.ignoreMissing = true
		case optStrict:
			set.strict = true
		case optTag:
			set.tag = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return settings{}, err
	}
	set.operands = operands
	return set, nil
}

// validate reports the options the utility refuses to take together, with
// the diagnostics the Perl script writes.
func (s settings) validate() error {
	switch {
	case s.binaryGiven && s.textGiven:
		return errors.New("Ambiguous file mode")
	case s.warn && !s.check:
		return errors.New("--warn option used only when verifying checksums")
	case s.status && !s.check:
		return errors.New("--status option used only when verifying checksums")
	case s.quiet && !s.check:
		return errors.New("--quiet option used only when verifying checksums")
	case s.ignoreMissing && !s.check:
		return errors.New("--ignore-missing option used only when verifying checksums")
	case s.strict && !s.check:
		return errors.New("--strict option used only when verifying checksums")
	case s.tag && s.textGiven:
		return errors.New("--tag does not support --text mode")
	}
	return nil
}

// usage describes the options of a family utility.
func usage(prog string) string {
	return `Usage: ` + prog + ` [-bcqstw] [-a algorithm] [file ...]

  -a, --algorithm=algorithm
            1 (default), 224, 256, 384, 512, 512224, 512256
  -b, --binary
            read in binary mode
  -c, --check
            read SHA sums from the files and check them
      --ignore-missing
            don't fail or report status for missing files
  -q, --quiet
            don't print OK for each successfully verified file
  -s, --status
            don't output anything, status code shows success
      --strict
            exit non-zero for improperly formatted checksum lines
      --tag
            create a BSD-style checksum
  -t, --text
            read in text mode (default)
  -w, --warn
            warn about improperly formatted checksum lines
      --help     display this help and exit
      --version  output version information and exit
`
}
