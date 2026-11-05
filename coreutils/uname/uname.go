// Package uname implements the GNU uname utility.
//
// The information comes from the host through the standard library: uname(2)
// on Linux, the kernel's sysctl interface on darwin and the BSDs, and the
// runtime plus the Windows version API on the other platforms. A field the
// platform cannot provide stays empty: it prints "unknown" when its option
// is given explicitly, as GNU does for -p and -i, and is omitted from the
// -a line.
package uname

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// unknown is what an explicitly requested but unavailable field prints.
const unknown = "unknown"

// Prog is the uname core utility.
type Prog struct {
	*coreutils.Runtime

	// collect gathers the system information of the host. When nil,
	// hostFields is used.
	collect func() (fields, error)
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the selected system information to the standard output, as a
// single line, and exits with status 1 on usage, collection and write
// errors.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "uname", usage, optSet, args); done {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	opts, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "uname")
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	info, err := p.hostInfo()
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if _, err := io.WriteString(p.Stdout, info.line(opts)+"\n"); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// hostInfo returns the system information of the host, honoring collect
// when the tests replaced it.
func (p *Prog) hostInfo() (fields, error) {
	if p.collect != nil {
		return p.collect()
	}
	return hostFields()
}

// fields holds the pieces of system information uname writes, in the order
// they appear on the output line.
type fields struct {
	sysname          string // -s, the kernel name
	nodename         string // -n
	release          string // -r
	version          string // -v
	machine          string // -m
	processor        string // -p
	hardwarePlatform string // -i
	osName           string // -o
}

// line joins the selected information with single spaces, in the order the
// GNU utility writes them, defaulting to the system name when no option
// selects one. An empty field is unavailable: it prints "unknown" when its
// option is given explicitly, as GNU prints for -p and -i, and is omitted
// from the -a line.
func (f fields) line(opts options) string {
	if opts.all {
		opts = options{
			all:     true,
			sysname: true, nodename: true, release: true, version: true,
			machine: true, processor: true, platform: true, osName: true,
		}
	} else if !opts.selected() {
		opts.sysname = true
	}
	wanted := []struct {
		requested bool
		value     string
	}{
		{opts.sysname, f.sysname},
		{opts.nodename, f.nodename},
		{opts.release, f.release},
		{opts.version, f.version},
		{opts.machine, f.machine},
		{opts.processor, f.processor},
		{opts.platform, f.hardwarePlatform},
		{opts.osName, f.osName},
	}
	var parts []string
	for _, w := range wanted {
		if !w.requested {
			continue
		}
		if w.value == "" {
			if opts.all {
				continue
			}
			w.value = unknown
		}
		parts = append(parts, w.value)
	}
	return strings.Join(parts, " ")
}

// options holds the pieces of information the options select.
type options struct {
	all       bool // -a
	sysname   bool // -s
	nodename  bool // -n
	release   bool // -r
	version   bool // -v
	machine   bool // -m
	processor bool // -p
	platform  bool // -i
	osName    bool // -o
}

// selected reports whether any option selects a piece of information.
func (o options) selected() bool {
	return o.all || o.sysname || o.nodename || o.release || o.version ||
		o.machine || o.processor || o.platform || o.osName
}

// longOptions maps the long options of uname to the short ones they stand
// for.
var longOptions = map[string]rune{
	"all":               'a',
	"kernel-name":       's',
	"nodename":          'n',
	"kernel-release":    'r',
	"kernel-version":    'v',
	"machine":           'm',
	"processor":         'p',
	"hardware-platform": 'i',
	"operating-system":  'o',
}

// optSet describes the options of uname.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of uname.
const usage = `Usage: uname [OPTION]...
Print certain system information. With no OPTION, same as -s.

  -a, --all
         print all information, in the order -s -n -r -v -m -p -i -o, omitting
         -p and -i when unknown
  -s, --kernel-name
         print the kernel name
  -n, --nodename
         print the network node hostname
  -r, --kernel-release
         print the kernel release
  -v, --kernel-version
         print the kernel version
  -m, --machine
         print the machine hardware name
  -p, --processor
         print the processor type (non-portable)
  -i, --hardware-platform
         print the hardware platform (non-portable)
  -o, --operating-system
         print the operating system
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the options. uname takes no operands.
func parseFlags(args []string) (options, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'a':
			opts.all = true
		case 's':
			opts.sysname = true
		case 'n':
			opts.nodename = true
		case 'r':
			opts.release = true
		case 'v':
			opts.version = true
		case 'm':
			opts.machine = true
		case 'p':
			opts.processor = true
		case 'i':
			opts.platform = true
		case 'o':
			opts.osName = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, err
	}
	if len(operands) > 0 {
		return options{}, fmt.Errorf("extra operand '%s'", operands[0])
	}
	return opts, nil
}

// hostname returns the host name of the machine, or empty when it cannot
// be determined.
func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// goarchMachine names the hardware the program was built for, as the
// machine names of the Unix systems usually spell it.
func goarchMachine(arch string) string {
	switch arch {
	case "amd64":
		return "x86_64"
	case "386":
		return "i686"
	default:
		return arch
	}
}

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	if p.Stderr == nil {
		return
	}
	fmt.Fprintf(p.Stderr, "uname: "+format+"\n", args...)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
