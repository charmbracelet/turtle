// Package getopt implements the POSIX option syntax shared by the core
// utilities: single-character options which may be grouped, option-arguments
// attached or held by the next argument, "--" ending the options, and parsing
// stopping at the first operand. Long options, a GNU extension, are accepted
// only when listed in a Set.
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
		operands []string
		parsing  = true
	)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case !parsing || arg == "-" || !strings.HasPrefix(arg, "-"):
			parsing = false
			operands = append(operands, arg)
		case arg == "--":
			parsing = false
		case strings.HasPrefix(arg, "--"):
			if err := s.parseLong(arg, args, &i, visit); err != nil {
				return nil, err
			}
		default:
			if err := s.parseShort(arg, args, &i, visit); err != nil {
				return nil, err
			}
		}
	}
	return operands, nil
}

// parseShort reports the options grouped in arg to visit, taking the value of
// a valued option from the rest of the group or from the next argument.
func (s Set) parseShort(arg string, args []string, i *int, visit Visit) error {
	for j := 1; j < len(arg); j++ {
		opt := rune(arg[j])
		if !s.valued(opt) && !s.optional(opt) {
			if err := visit(opt, ""); err != nil {
				return err
			}
			continue
		}
		value := arg[j+1:]
		if value == "" {
			if *i+1 == len(args) {
				return fmt.Errorf("option requires an argument -- '%c'", opt)
			}
			*i++
			value = args[*i]
		}
		return visit(opt, value)
	}
	return nil
}

// parseLong reports the long option held in arg to visit, as the character it
// stands for.
func (s Set) parseLong(arg string, args []string, i *int, visit Visit) error {
	name, value, given := strings.Cut(arg[2:], "=")
	opt, ok := s.Long[name]
	if !ok {
		return fmt.Errorf("unrecognized option '%s'", arg)
	}
	switch {
	case s.attached(opt):
		if !given {
			return visit(opt, "")
		}
		return visit(opt, value)
	case s.optional(opt):
		if !given {
			return visit(opt, "")
		}
		return visit(opt, value)
	case !s.valued(opt):
		if given {
			return fmt.Errorf("option '%s' doesn't allow an argument", arg)
		}
		return visit(opt, "")
	}
	if !given {
		if *i+1 == len(args) {
			return fmt.Errorf("option '%s' requires an argument", arg)
		}
		*i++
		value = args[*i]
	}
	return visit(opt, value)
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

// InvalidOption builds the error of an option which a program doesn't take.
func InvalidOption(opt rune) error {
	return fmt.Errorf("invalid option -- '%c'", opt)
}
