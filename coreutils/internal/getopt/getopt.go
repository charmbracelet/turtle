// Package getopt implements the POSIX option syntax shared by the core
// utilities: single-character options which may be grouped, option-arguments
// attached or held by the next argument, "--" ending the options, and parsing
// stopping at the first operand. Long options, a GNU extension, are accepted
// only when listed in a Set. The options can also follow the operands, and a
// collecting option can take every argument below it, when a Set asks for
// it, as zip and unzip do.
package getopt

import (
	"fmt"
	"strings"
)

// Set describes the options taken by a program.
type Set struct {
	// Valued holds the option characters which require an argument, like the
	// "n" of head. In a group, the rest of the argument is the value, as in
	// "-n5", or the next argument is taken, as in "-n 5".
	Valued string

	// Optional holds the option characters which may take an argument, like
	// the "p" of mktemp. The short form requires it, as in "-p dir", while
	// the long form only takes it when attached, as in "--tmpdir=dir", and
	// an empty value is allowed, as in "--tmpdir=" or "--tmpdir".
	Optional string

	// Attached holds the option characters which take an argument only when
	// attached to their long form with "=", as in "--follow=name". The short
	// form takes none, as in "-f", and the bare long form doesn't either, as
	// in "--follow".
	Attached string

	// Collect holds the option characters whose arguments are the ones
	// below them, without stopping at the next dashed argument, as the
	// -x and -i of zip: Parse reports each following argument which isn't
	// an option to the same option, until a dashed one, "--" or a lone
	// "@" closes the collection. An attached value, like in "-xpat" or
	// "--exclude=pat", is the only one, since such lists need a space
	// after the option. A collecting character is never held by Valued,
	// Optional or Attached.
	Collect string

	// Permute lets the options follow the operands, as GNU getopt does:
	// Parse keeps treating the dashed arguments below an operand as
	// options, keeping the operands in the order given. Without it, the
	// POSIX syntax stops parsing at the first operand.
	Permute bool

	// Long maps the long options of a program to the characters they stand
	// for. When nil, every argument starting with "--" is rejected.
	Long map[string]rune
}

// Visit is the function which receives each parsed option, with the argument
// it takes, or an empty value otherwise.
type Visit func(opt rune, value string) error

// Parse reports each option of args to visit, in the order given, and returns
// the operands left after them. An error from visit stops the parsing and is
// returned as is.
func (s Set) Parse(args []string, visit Visit) ([]string, error) {
	var (
		operands   []string
		parsing    = true
		collecting rune
		ended      bool
	)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--" && parsing:
			collecting = 0
			parsing = false
			ended = true
		case ended:
			operands = append(operands, arg)
		case arg == "@" && collecting != 0:
			collecting = 0
		case !strings.HasPrefix(arg, "-") && collecting != 0:
			if err := visit(collecting, arg); err != nil {
				return nil, err
			}
		case arg == "-" || !strings.HasPrefix(arg, "-") || !parsing:
			operands = append(operands, arg)
			collecting = 0
			if !s.Permute {
				parsing = false
			}
		case strings.HasPrefix(arg, "--"):
			opened, err := s.parseLong(arg, args, &i, visit)
			if err != nil {
				return nil, err
			}
			collecting = opened
		default:
			opened, err := s.parseShort(arg, args, &i, visit)
			if err != nil {
				return nil, err
			}
			collecting = opened
		}
	}
	return operands, nil
}

// parseShort reports the options grouped in arg to visit, taking the value of
// a valued option from the rest of the group or from the next argument, and
// returns the character which collects the arguments below the group, or 0.
func (s Set) parseShort(arg string, args []string, i *int, visit Visit) (rune, error) {
	for j := 1; j < len(arg); j++ {
		opt := rune(arg[j])
		if s.collects(opt) {
			if rest := arg[j+1:]; rest != "" {
				return 0, visit(opt, rest)
			}
			return opt, nil
		}
		if !s.valued(opt) && !s.optional(opt) {
			if err := visit(opt, ""); err != nil {
				return 0, err
			}
			continue
		}
		value := arg[j+1:]
		if value == "" {
			if *i+1 == len(args) {
				return 0, fmt.Errorf("option requires an argument -- '%c'", opt)
			}
			*i++
			value = args[*i]
		}
		return 0, visit(opt, value)
	}
	return 0, nil
}

// parseLong reports the long option held in arg to visit, as the character it
// stands for, and returns the character which collects the arguments below
// it, or 0.
func (s Set) parseLong(arg string, args []string, i *int, visit Visit) (rune, error) {
	name, value, given := strings.Cut(arg[2:], "=")
	opt, ok := s.Long[name]
	if !ok {
		return 0, fmt.Errorf("unrecognized option '%s'", arg)
	}
	switch {
	case s.collects(opt):
		// Without a space before the value the list has just one.
		if given {
			return 0, visit(opt, value)
		}
		return opt, nil
	case s.attached(opt):
		if !given {
			return 0, visit(opt, "")
		}
		return 0, visit(opt, value)
	case s.optional(opt):
		if !given {
			return 0, visit(opt, "")
		}
		return 0, visit(opt, value)
	case !s.valued(opt):
		if given {
			return 0, fmt.Errorf("option '%s' doesn't allow an argument", arg)
		}
		return 0, visit(opt, "")
	}
	if !given {
		if *i+1 == len(args) {
			return 0, fmt.Errorf("option '%s' requires an argument", arg)
		}
		*i++
		value = args[*i]
	}
	return 0, visit(opt, value)
}

func (s Set) valued(opt rune) bool {
	return strings.ContainsRune(s.Valued, opt)
}

func (s Set) optional(opt rune) bool {
	return strings.ContainsRune(s.Optional, opt)
}

func (s Set) attached(opt rune) bool {
	return strings.ContainsRune(s.Attached, opt)
}

func (s Set) collects(opt rune) bool {
	return strings.ContainsRune(s.Collect, opt)
}

// InvalidOption builds the error of an option which a program doesn't take.
func InvalidOption(opt rune) error {
	return fmt.Errorf("invalid option -- '%c'", opt)
}
