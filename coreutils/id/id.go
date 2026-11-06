// Package id implements the POSIX id utility.
//
// The identity of the current process comes from the host, through the
// standard library's os/user and os packages: the real and effective IDs
// are the same, so -r is accepted and without effect, and the names
// resolve through the host user database, printing the numeric IDs when
// they don't. A user operand resolves through the same database, as a
// name or a numeric user ID, without the group memberships, which the
// standard library cannot enumerate.
package id

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the id core utility.
type Prog struct {
	*coreutils.Runtime

	// current returns the identity of the host process. When nil,
	// hostIdentity is used.
	current func() (idInfo, error)

	// lookup resolves a user operand through the host database. When
	// nil, hostLookup is used.
	lookup func(string) (idInfo, bool)
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the identity of the host process, or of the user the
// operand names, to the standard output, as the options select, and
// exits with status 1 on usage, identity and write errors.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "id", usage, optSet, args); done {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	opts, operand, given, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "id")
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	var in idInfo
	if given {
		var ok bool
		in, ok = p.lookupOperand(operand)
		if !ok {
			err := fmt.Errorf("'%s': no such user", operand)
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	} else if in, err = p.identity(); err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return p.write(in, opts)
}

// write outputs the identity to the standard output, as the options
// select.
func (p *Prog) write(in idInfo, opts options) error {
	if _, err := io.WriteString(p.Stdout, in.output(opts)); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// identity returns the identity of the host process, honoring current when
// the tests replaced it.
func (p *Prog) identity() (idInfo, error) {
	if p.current != nil {
		return p.current()
	}
	return hostIdentity()
}

// lookupOperand resolves a user operand, honoring lookup when the tests
// replaced it.
func (p *Prog) lookupOperand(spec string) (idInfo, bool) {
	if p.lookup != nil {
		return p.lookup(spec)
	}
	return hostLookup(spec)
}

// namedID is a user or group ID, with the name it resolves to in the host
// database, or empty when it doesn't.
type namedID struct {
	id   string
	name string
}

// value returns the name when it resolves and names are asked for, or the
// ID otherwise.
func (n namedID) value(name bool) string {
	if name && n.name != "" {
		return n.name
	}
	return n.id
}

// tagged returns the ID with its name in parentheses, or the ID alone when
// the name doesn't resolve.
func (n namedID) tagged() string {
	if n.name == "" {
		return n.id
	}
	return n.id + "(" + n.name + ")"
}

// idInfo is the identity of the process: its user and group IDs, with the
// supplementary groups the host reports, the primary group included when
// the platform lists it there, as getgroups does on the Unix systems.
type idInfo struct {
	user   namedID
	group  namedID
	groups []namedID
}

// line returns the default uid=/gid=/groups= line, with the effective
// values: the real ones are the same, so they are never listed apart.
func (in idInfo) line() string {
	parts := []string{
		"uid=" + in.user.tagged(),
		"gid=" + in.group.tagged(),
	}
	if len(in.groups) > 0 {
		groups := make([]string, len(in.groups))
		for i, g := range in.groups {
			groups[i] = g.tagged()
		}
		parts = append(parts, "groups="+strings.Join(groups, ","))
	}
	return strings.Join(parts, " ")
}

// list returns the group IDs of -G: the primary one first, then the
// supplementary ones which differ from it.
func (in idInfo) list() []namedID {
	out := []namedID{in.group}
	for _, g := range in.groups {
		if g.id != in.group.id {
			out = append(out, g)
		}
	}
	return out
}

// output returns what the options select from the identity, NUL-delimited
// with -z, newline-delimited otherwise.
func (in idInfo) output(opts options) string {
	delim, end := " ", "\n"
	if opts.zero {
		delim, end = "\x00", "\x00"
	}
	items := []string{in.line()}
	switch {
	case opts.user:
		items = []string{in.user.value(opts.name)}
	case opts.group:
		items = []string{in.group.value(opts.name)}
	case opts.groupList:
		groups := in.list()
		items = make([]string, len(groups))
		for i, g := range groups {
			items[i] = g.value(opts.name)
		}
	}
	return strings.Join(items, delim) + end
}

// userInfo returns the identity of a user of the host database: the user
// and group IDs, with the names which resolve.
func userInfo(u *user.User) idInfo {
	in := idInfo{
		user:  namedID{id: u.Uid, name: u.Username},
		group: namedID{id: u.Gid},
	}
	if g, err := user.LookupGroupId(u.Gid); err == nil {
		in.group.name = g.Name
	}
	return in
}

// hostIdentity returns the identity of the host process. IDs are the
// numeric strings the Unix systems report; on Windows they are the SID
// strings instead, and the supplementary groups are unavailable and
// omitted.
func hostIdentity() (idInfo, error) {
	u, err := user.Current()
	if err != nil {
		return idInfo{}, fmt.Errorf("cannot get user identity: %w", err)
	}
	in := userInfo(u)
	gids, err := os.Getgroups()
	if err != nil {
		return in, nil
	}
	for _, gid := range gids {
		g := namedID{id: strconv.Itoa(gid)}
		if grp, err := user.LookupGroupId(g.id); err == nil {
			g.name = grp.Name
		}
		in.groups = append(in.groups, g)
	}
	return in, nil
}

// hostLookup resolves a user operand through the host database: a name
// first, then a numeric user ID. It reports false when neither resolves.
// The group memberships of the selected user cannot be enumerated with
// the standard library, so they are left out: the default format prints
// only the uid and gid, and -G prints only the primary group ID.
func hostLookup(spec string) (idInfo, bool) {
	for _, lookup := range []func(string) (*user.User, error){
		user.Lookup, user.LookupId,
	} {
		if u, err := lookup(spec); err == nil {
			return userInfo(u), true
		}
	}
	return idInfo{}, false
}

// options holds the pieces of information the options select.
type options struct {
	user      bool // -u
	group     bool // -g
	groupList bool // -G
	name      bool // -n
	real      bool // -r
	zero      bool // -z
}

// selected reports whether one of -u, -g or -G selects a single piece of
// information, leaving the default format otherwise.
func (o options) selected() bool {
	return o.user || o.group || o.groupList
}

// check rejects the option combinations the utility doesn't take, with
// the diagnostics GNU writes.
func (o options) check() error {
	if o.selected() {
		n := 0
		for _, ok := range []bool{o.user, o.group, o.groupList} {
			if ok {
				n++
			}
		}
		if n > 1 {
			return errors.New(`cannot print "only" of more than one choice`)
		}
		return nil
	}
	if o.name || o.real {
		return errors.New("printing only names or real IDs requires -u, -g, or -G")
	}
	if o.zero {
		return errors.New("option --zero not permitted in default format")
	}
	return nil
}

// longOptions maps the long options of id to the short ones they stand
// for.
var longOptions = map[string]rune{
	"group":  'g',
	"groups": 'G',
	"name":   'n',
	"real":   'r',
	"user":   'u',
	"zero":   'z',
}

// optSet describes the options of id.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of id.
const usage = `Usage: id [OPTION]... [USER]
Print user and group information for USER, or for the current
process when it is omitted.

  -g, --group
         print only the effective group ID
  -G, --groups
         print all group IDs
  -n, --name
         print a name instead of a number, for -u, -g, -G
  -r, --real
         print the real ID instead of the effective ID, with -u, -g, -G
  -u, --user
         print only the effective user ID
  -z, --zero
         delimit entries with NUL characters, not whitespace;
         not permitted in default format
      --help
         display this help and exit
      --version
         output version information and exit

Without any OPTION, print some useful set of identified information.
`

// parseFlags parses the options, returning the user operand and whether
// one was given, at most one. An empty operand is given: it is looked up
// like any other, and no user resolves from it.
func parseFlags(args []string) (options, string, bool, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'g':
			opts.group = true
		case 'G':
			opts.groupList = true
		case 'n':
			opts.name = true
		case 'r':
			opts.real = true
		case 'u':
			opts.user = true
		case 'z':
			opts.zero = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, "", false, err
	}
	if err := opts.check(); err != nil {
		return options{}, "", false, err
	}
	if len(operands) > 1 {
		return options{}, "", false, fmt.Errorf("extra operand '%s'", operands[1])
	}
	if len(operands) == 1 {
		return opts, operands[0], true, nil
	}
	return opts, "", false, nil
}

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	if p.Stderr == nil {
		return
	}
	fmt.Fprintf(p.Stderr, "id: "+format+"\n", args...)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
