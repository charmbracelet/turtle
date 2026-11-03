package ls

import (
	"fmt"
	"strconv"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/sizefmt"
)

// followMode is how symbolic links are followed.
type followMode int

const (
	followDefault  followMode = iota // only operand links to directories
	followOperands                   // -H
	followAlways                     // -L
)

// sortKey is what a listing is sorted by.
type sortKey int

const (
	sortName      sortKey = iota
	sortSize              // -S
	sortTime              // -t
	sortNone              // -f, -U
	sortExtension         // -X
	sortVersion           // -v
	sortWidth             // --sort=width
)

// timeKey is the time a listing is written and sorted with.
type timeKey int

const (
	timeModified timeKey = iota
	timeStatus           // -c
	timeAccessed         // -u
	timeBirth            // --time=birth
)

// layout is the format of a listing that isn't long.
type layout int

const (
	layoutOne     layout = iota // one entry per line, the default and -1
	layoutColumns               // -C
	layoutAcross                // -x
	layoutStream                // -m
)

// classify is the file type indicator written after a name.
type classify byte

const (
	classifyNone classify = iota // no indicator
	classifyDirs                 // -p, --indicator-style=slash: directories only
	classifyFile                 // --file-type: everything but executables
	classifyFull                 // -F, --classify: everything
)

// howControlChars says what happens to the nongraphic characters a
// quoting style would write as they are.
type howControlChars int

const (
	hideDefault howControlChars = iota // hide them on a terminal, show otherwise
	hideAlways                         // -q: write them as question marks
	showAlways                         // --show-control-chars: write them
)

// options are the listing options parsed from the command line.
type options struct {
	all       bool // -A/-a: write hidden entries too
	fast      bool // -f: keep the order of the directory
	inode     bool // -i: write the file serial number
	blocks    bool // -s: write the block count
	kilo      bool // -k: count blocks in kibibytes
	recursive bool // -R: list subdirectories
	dirs      bool // -d: write directories as they are
	reverse   bool // -r: reverse the order
	long      bool // -l/-g/-n/-o: long format
	owner     bool // write the owner
	group     bool // write the group
	author    bool // --author: write the author too
	numeric   bool // -n: write identifiers as numbers
	groupDirs bool // --group-directories-first
	zero      bool // --zero: end lines with NUL

	ignore []string // --ignore: patterns of names dropped from directories
	hide   []string // --hide: patterns dropped unless -a or -A was given

	follow        followMode
	followArgDirs bool // --dereference-command-line-symlink-to-dir
	sort          sortKey
	time          timeKey
	layout        layout
	layoutSet     bool // whether a layout option was given
	classify      classify

	quoting    quotingStyle
	quotingSet bool
	hideCtrl   howControlChars

	blockSize    sizefmt.Spec // --block-size, -h and --si
	blockSizeSet bool
	timeStyle    string // the word --time-style stands for, empty when none
	tabsize      int    // -T, negative when unset
	width        int    // -w, negative when unset
}

// set turns on the option given, applying the argument of a valued long
// option.
func (o *options) set(opt rune, value string) error {
	switch opt {
	case 'A', 'a':
		o.all = true
	case 'B':
		o.ignore = append(o.ignore, "*~", ".*~")
	case 'C':
		o.layout, o.layoutSet, o.long = layoutColumns, true, false
	case 'F':
		o.classify = classifyFull
	case 'G':
		o.group = false
	case 'H':
		o.follow = followOperands
	case 'I':
		o.ignore = append(o.ignore, value)
	case 'L':
		o.follow = followAlways
	case 'R':
		o.recursive, o.dirs = true, false
	case 'S':
		o.sort = sortSize
	case 'T':
		return o.setTabsize(value)
	case 'U':
		o.sort = sortNone
	case 'X':
		o.sort = sortExtension
	case 'b':
		o.quoting, o.quotingSet = quoteEscape, true
	case 'c':
		o.time = timeStatus
	case 'd':
		o.dirs, o.recursive = true, false
	case 'f':
		o.fast, o.all = true, true
	case 'g':
		o.long, o.owner = true, false
	case 'h':
		o.blockSize, o.blockSizeSet = sizefmt.HumanSpec(false), true
	case 'i':
		o.inode = true
	case 'k':
		o.kilo = true
	case 'l':
		o.long = true
	case 'm':
		o.layout, o.layoutSet, o.long = layoutStream, true, false
	case 'n':
		o.long, o.numeric = true, true
	case 'N':
		o.quoting, o.quotingSet = quoteLiteral, true
	case 'o':
		o.long, o.group = true, false
	case 'p':
		o.classify = classifyDirs
	case 'q':
		o.hideCtrl = hideAlways
	case 'Q':
		o.quoting, o.quotingSet = quoteC, true
	case 'r':
		o.reverse = true
	case 's':
		o.blocks = true
	case 't':
		o.sort = sortTime
	case 'u':
		o.time = timeAccessed
	case 'v':
		o.sort = sortVersion
	case 'w':
		return o.setWidth(value)
	case 'x':
		o.layout, o.layoutSet, o.long = layoutAcross, true, false
	case '1':
		// unlike --format=single-column, -1 keeps the long format
		o.layout, o.layoutSet = layoutOne, true
	case optAuthor:
		o.author = true
	case optBlockSize:
		spec, err := sizefmt.Parse(value)
		if err != nil {
			return err
		}
		o.blockSize, o.blockSizeSet = spec, true
	case optDerefArgDirs:
		o.followArgDirs = true
	case optFormat:
		return o.setFormat(value)
	case optFileType:
		o.classify = classifyFile
	case optFullTime:
		o.long, o.timeStyle = true, "full-iso"
	case optGroupDirs:
		o.groupDirs = true
	case optHide:
		o.hide = append(o.hide, value)
	case optIndicatorStyle:
		return o.setIndicatorStyle(value)
	case optQuotingStyle:
		return o.setQuotingStyle(value)
	case optShowCtrl:
		o.hideCtrl = showAlways
	case optSi:
		o.blockSize, o.blockSizeSet = sizefmt.HumanSpec(true), true
	case optSort:
		return o.setSort(value)
	case optTime:
		return o.setTime(value)
	case optTimeStyle:
		o.timeStyle = value
	case optZero:
		o.zero = true
		o.hideCtrl = showAlways
		if !o.long {
			o.layout, o.layoutSet = layoutOne, true
		}
		o.quoting, o.quotingSet = quoteLiteral, true
	default:
		return getopt.InvalidOption(opt)
	}
	return nil
}

// setTabsize applies the argument of --tabsize: the distance between the
// tab stops the column formats align within, zero to use none.
func (o *options) setTabsize(value string) error {
	size, err := strconv.Atoi(value)
	if err != nil || size < 0 {
		return fmt.Errorf("invalid tab size '%s'", value)
	}
	o.tabsize = size
	return nil
}

// setWidth applies the argument of --width: the width the column formats
// write within, zero for none.
func (o *options) setWidth(value string) error {
	width, err := strconv.Atoi(value)
	if err != nil || width < 0 {
		return fmt.Errorf("invalid line width '%s'", value)
	}
	o.width = width
	return nil
}

// setFormat applies the word --format stands for: the layout it selects.
func (o *options) setFormat(word string) error {
	switch word {
	case "long", "verbose":
		o.long, o.layoutSet = true, true
	case "vertical":
		o.layout, o.layoutSet, o.long = layoutColumns, true, false
	case "across", "horizontal":
		o.layout, o.layoutSet, o.long = layoutAcross, true, false
	case "commas":
		o.layout, o.layoutSet, o.long = layoutStream, true, false
	case "single-column":
		o.layout, o.layoutSet, o.long = layoutOne, true, false
	default:
		return invalidWord("format", word)
	}
	return nil
}

// setIndicatorStyle applies the word --indicator-style stands for: the file
// type indicator written after a name.
func (o *options) setIndicatorStyle(word string) error {
	switch word {
	case "none":
		o.classify = classifyNone
	case "slash":
		o.classify = classifyDirs
	case "file-type":
		o.classify = classifyFile
	case "classify":
		o.classify = classifyFull
	default:
		return invalidWord("indicator-style", word)
	}
	return nil
}

// setQuotingStyle applies the word --quoting-style stands for.
func (o *options) setQuotingStyle(word string) error {
	style, ok := quotingStyles[word]
	if !ok {
		return invalidWord("quoting-style", word)
	}
	o.quoting, o.quotingSet = style, true
	return nil
}

// setSort applies the word --sort stands for: the key a listing is sorted by.
func (o *options) setSort(word string) error {
	switch word {
	case "name":
		o.sort = sortName
	case "time":
		o.sort = sortTime
	case "size":
		o.sort = sortSize
	case "none":
		o.sort = sortNone
	case "extension":
		o.sort = sortExtension
	case "version":
		o.sort = sortVersion
	case "width":
		o.sort = sortWidth
	default:
		return invalidWord("sort", word)
	}
	return nil
}

// setTime applies the word --time stands for: the time a listing is written
// and sorted with.
func (o *options) setTime(word string) error {
	switch word {
	case "atime", "access", "use":
		o.time = timeAccessed
	case "ctime", "status":
		o.time = timeStatus
	case "mtime", "modification":
		o.time = timeModified
	case "birth", "creation":
		o.time = timeBirth
	default:
		return invalidWord("time", word)
	}
	return nil
}

// invalidWord builds the error of a word a valued long option doesn't take.
func invalidWord(option, word string) error {
	return fmt.Errorf("invalid argument '%s' for '--%s'", word, option)
}

// The long options which have no short equivalent, as runes above the
// bytes a group of short options can hold, keeping them out of the short
// syntax.
const (
	optFormat rune = 0x100 + iota
	optIndicatorStyle
	optSort
	optTime
	optFileType
	optAuthor
	optBlockSize
	optDerefArgDirs
	optFullTime
	optGroupDirs
	optHide
	optQuotingStyle
	optShowCtrl
	optSi
	optTimeStyle
	optZero
)

// longOptions maps the long options of ls to the short ones they stand
// for, or to the runes of the ones with no short equivalent.
var longOptions = map[string]rune{
	"all":                      'a',
	"almost-all":               'A',
	"author":                   optAuthor,
	"block-size":               optBlockSize,
	"classify":                 'F',
	"dereference":              'L',
	"dereference-command-line": 'H',
	"dereference-command-line-symlink-to-dir": optDerefArgDirs,
	"directory":               'd',
	"escape":                  'b',
	"file-type":               optFileType,
	"format":                  optFormat,
	"full-time":               optFullTime,
	"group-directories-first": optGroupDirs,
	"hide":                    optHide,
	"hide-control-chars":      'q',
	"human-readable":          'h',
	"ignore":                  'I',
	"ignore-backups":          'B',
	"indicator-style":         optIndicatorStyle,
	"inode":                   'i',
	"kibibytes":               'k',
	"literal":                 'N',
	"no-group":                'G',
	"numeric-uid-gid":         'n',
	"quote-name":              'Q',
	"quoting-style":           optQuotingStyle,
	"recursive":               'R',
	"reverse":                 'r',
	"show-control-chars":      optShowCtrl,
	"si":                      optSi,
	"size":                    's',
	"sort":                    optSort,
	"tabsize":                 'T',
	"time":                    optTime,
	"time-style":              optTimeStyle,
	"width":                   'w',
	"zero":                    optZero,
}

// valuedLong is the Valued set of the long options which take a word: the
// ones with no short equivalent.
const valuedLong = string(optFormat) + string(optIndicatorStyle) +
	string(optSort) + string(optTime) + string(optHide) +
	string(optBlockSize) + string(optQuotingStyle) + string(optTimeStyle)

// optSet describes the options of ls.
var optSet = getopt.Set{
	Valued: "ITw" + valuedLong,
	Long:   longOptions,
}

// usage describes the options of ls.
const usage = `Usage: ls [OPTION]... [FILE]...
List information about the FILEs (the current directory by default).
Sort entries alphabetically if none of -cftuvSUX nor --sort is specified.

  -a, --all
         do not ignore entries starting with .
  -A, --almost-all
         do not list implied . and ..
      --author
         with -l, print the author of each file
  -b, --escape
         print C-style escapes for nongraphic characters
      --block-size=SIZE
         scale sizes by SIZE when printing them;
         e.g., '--block-size=M'; see SIZE format below
  -B, --ignore-backups
         do not list implied entries ending with ~
  -c
         with -lt: sort by, and show, ctime;
         with -l: show ctime and sort by name;
         otherwise: sort by ctime, newest first
  -C
         list entries by columns
  -d, --directory
         list directories themselves, not their contents
  -f
         do not sort, enable -aU, disable -l --color
  -F, --classify
         append indicator (one of */=>@|) to entries
      --file-type
         likewise, except do not append '*'
      --format=WORD
         across -x, commas -m, horizontal -x, long -l,
         single-column -1, verbose -l, vertical -C
      --full-time
         like -l --time-style=full-iso
  -g
         like -l, but do not list owner
      --group-directories-first
         group directories before files;
         can be augmented with a --sort option, but any
         use of --sort=none (-U) disables grouping
  -G, --no-group
         in a long listing, don't print group names
  -h, --human-readable
         print sizes like 1K 234M 2G etc.
      --si
         likewise, but use powers of 1000 not 1024
  -H, --dereference-command-line
         follow symbolic links listed on the command line
      --dereference-command-line-symlink-to-dir
         follow each command line symbolic link
         that points to a directory
      --hide=PATTERN
         do not list implied entries matching shell PATTERN
         (overridden by -a or -A)
      --indicator-style=WORD
         append indicator with style WORD to entry names:
         none (default), slash (-p),
         file-type (--file-type), classify (-F)
  -i, --inode
         print the index number of each file
  -I, --ignore=PATTERN
         do not list implied entries matching shell PATTERN
  -k, --kibibytes
         default to 1024-byte blocks for file system usage;
         used only with -s and per directory totals
  -l
         use a long listing format
  -L, --dereference
         when showing file information for a symbolic link,
         show information for the file the link references
         rather than for the link itself
  -m
         fill width with a comma separated list of entries
  -n, --numeric-uid-gid
         like -l, but list numeric user and group IDs
  -N, --literal
         print entry names without quoting
  -o
         like -l, but do not list group information
  -p, --indicator-style=slash
         append / indicator to directories
  -q, --hide-control-chars
         print ? instead of nongraphic characters
      --show-control-chars
         print nongraphic characters as-is (the default,
         unless program is 'ls' and output is a terminal)
  -Q, --quote-name
         enclose entry names in double quotes
      --quoting-style=WORD
         use quoting style WORD for entry names:
         literal, locale, shell, shell-always,
         shell-escape, shell-escape-always, c, escape
         (overrides QUOTING_STYLE environment variable)
  -r, --reverse
         reverse order while sorting
  -R, --recursive
         list subdirectories recursively
  -s, --size
         print the allocated size of each file, in blocks
  -S
         sort by file size, largest first
      --sort=WORD
         sort by WORD instead of name: none (-U), size (-S),
         time (-t), version (-v), extension (-X), width
      --time=WORD
         select which timestamp used to display or sort;
         access time (-u): atime, access, use;
         metadata change time (-c): ctime, status;
         birth time: birth, creation;
         modified time (default): mtime, modification
      --time-style=TIME_STYLE
         time/date format with -l; see TIME_STYLE below
  -t
         sort by modification time, newest first
  -T, --tabsize=COLS
         assume tab stops at each COLS instead of 8
  -u
         with -lt: sort by, and show, access time;
         with -l: show access time and sort by name;
         otherwise: sort by access time, newest first
  -U
         do not sort; list entries in directory order
  -v
         natural sort of (version) numbers within text
  -w, --width=COLS
         set output width to COLS. 0 means no limit
  -x
         list entries by lines instead of by columns
  -X
         sort alphabetically by entry extension
  -1
         list one file per line
      --zero
         end each output line with NUL, not newline
      --help
         display this help and exit
      --version
         output version information and exit

SIZE is an integer and optional unit (e.g., 10K is 10*1024, 10KB is
10*1000). Units are K,M,G,T,P,E,Z,Y,R,Q (powers of 1024) or KB,MB,...
(powers of 1000), or KiB,MiB,... (powers of 1024).

TIME_STYLE is full-iso, long-iso, iso, locale, or +FORMAT, with FORMAT
interpreted as in the date utility. A "posix-" prefix uses the locale
form when LC_TIME is the POSIX locale.

The SIZE format and the environment variables COLUMNS, TABSIZE,
QUOTING_STYLE, TIME_STYLE, LS_BLOCK_SIZE, BLOCK_SIZE and POSIXLY_CORRECT
are as the GNU manual describes them.

Exit status:
 0  if OK,
 1  if minor problems (e.g., cannot access subdirectory),
 2  if serious trouble (e.g., cannot access command-line argument).
`

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{owner: true, group: true, tabsize: -1, width: -1}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		return opts.set(opt, value)
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}
