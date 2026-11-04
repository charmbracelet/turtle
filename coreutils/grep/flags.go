package grep

import (
	"fmt"
	"strings"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/glob"
)

// Long options with no short form stand for private runes.
const (
	optInclude rune = 0x100 + iota
	optExclude
	optExcludeDir
	optExcludeFrom
	optGroupSep
	optNoGroupSep
	optLabel
	optBinaryFiles
	optColor
	optNoIgnoreCase
	optLineBuffered
	optMmap
)

// optSet describes the options of grep.
var optSet = getopt.Set{
	Valued: "efmABCdD" +
		string(optBinaryFiles) + string(optExclude) + string(optExcludeDir) +
		string(optExcludeFrom) + string(optGroupSep) + string(optInclude) +
		string(optLabel),
	Optional: string(optColor),
	Long: map[string]rune{
		"after-context":         'A',
		"before-context":        'B',
		"binary-files":          optBinaryFiles,
		"basic-regexp":          'G',
		"context":               'C',
		"color":                 optColor,
		"colour":                optColor,
		"count":                 'c',
		"dereference-recursive": 'R',
		"devices":               'D',
		"directories":           'd',
		"exclude":               optExclude,
		"exclude-dir":           optExcludeDir,
		"exclude-from":          optExcludeFrom,
		"extended-regexp":       'E',
		"file":                  'f',
		"files-with-matches":    'l',
		"files-without-match":   'L',
		"fixed-strings":         'F',
		"group-separator":       optGroupSep,
		"ignore-case":           'i',
		"include":               optInclude,
		"initial-tab":           'T',
		"label":                 optLabel,
		"line-buffered":         optLineBuffered,
		"line-number":           'n',
		"line-regexp":           'x',
		"max-count":             'm',
		"mmap":                  optMmap,
		"no-filename":           'h',
		"no-group-separator":    optNoGroupSep,
		"no-ignore-case":        optNoIgnoreCase,
		"no-messages":           's',
		"null":                  'Z',
		"null-data":             'z',
		"only-matching":         'o',
		"quiet":                 'q',
		"recursive":             'r',
		"regexp":                'e',
		"revert-match":          'v',
		"silent":                'q',
		"text":                  'a',
		"word-regexp":           'w',
		"with-filename":         'H',
	},
}

// usage describes the options of grep.
const usage = `Usage: grep [OPTION]... PATTERNS [FILE]...
Search for PATTERNS in each FILE, or the standard input when no file is
given, or the working directory when recursing. PATTERNS is one or more
patterns separated by newline characters, and grep prints each line that
matches a pattern.

  -A NUM, --after-context=NUM
         print NUM lines of trailing context
  -a, --text
         process a binary file as if it were text
  -B NUM, --before-context=NUM
         print NUM lines of leading context
  -C NUM, --context=NUM
         print NUM lines of leading and trailing context
  -NUM
         same as --context=NUM
  -b, --byte-offset
         print the byte offset of each output line, of each match with -o
  -c, --count
         print only a count of selected lines
  -D ACTION, --devices=ACTION
         how to handle devices, FIFOs and sockets: read or skip
  -d ACTION, --directories=ACTION
         how to handle directories: read, skip or recurse
      --binary-files=TYPE
         how to handle binary files: binary, without-match or text
      --color[=WHEN], --colour[=WHEN]
         surround the matches and the prefixes with escape sequences;
         WHEN is always, never or auto
  -E, --extended-regexp
         PATTERNS are extended regular expressions
  -e PATTERNS, --regexp=PATTERNS
         use PATTERNS for matching
      --exclude=GLOB
         skip files whose name matches GLOB
      --exclude-dir=GLOB
         skip directories whose name matches GLOB
      --exclude-from=FILE
         skip files whose name matches a pattern of FILE
  -F, --fixed-strings
         PATTERNS are strings
  -f FILE, --file=FILE
         take PATTERNS from FILE
  -G, --basic-regexp
         PATTERNS are basic regular expressions
      --group-separator=SEP
         use SEP to separate groups of context lines
  -H, --with-filename
         print the file name for each match
  -h, --no-filename
         suppress the file name prefix
      --include=GLOB
         search only files whose name matches GLOB
  -I
         process a binary file as if it had no matching data
  -i, --ignore-case
         ignore case distinctions
  -L, --files-without-match
         print only the names of files with no selected lines
  -l, --files-with-matches
         print only the names of files with selected lines
      --label=LABEL
         take the name of the standard input as LABEL
      --line-buffered
         flush the output after every line
  -m NUM, --max-count=NUM
         stop after NUM selected lines
  -n, --line-number
         print the line number with output lines
      --mmap
         read the input directly, without buffering
      --no-group-separator
         print no separator between groups of context lines
      --no-ignore-case
         do not ignore case distinctions
  -o, --only-matching
         print only the matched parts of selected lines
  -q, --quiet, --silent
         suppress all output
  -R, --dereference-recursive
         read all files under each directory, following symbolic links
  -r, --recursive
         read all files under each directory, following the symbolic links
         of the command line only
  -s, --no-messages
         suppress error messages about nonexistent or unreadable files
  -T, --initial-tab
         make sure the line content starts at a tab stop
  -v, --revert-match
         select non-matching lines
  -w, --word-regexp
         select only matches that form whole words
  -x, --line-regexp
         match only whole lines
  -y
         obsolete synonym for -i
  -Z, --null
         print a null byte after the file name
  -z, --null-data
         separate the input and the output lines with null bytes
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the options of grep.
func parseFlags(args []string) (options, []string, error) {
	opts := options{mode: 'B', output: 'o', maxCount: -1, groupSep: "--"}
	operands, err := optSet.Parse(expandContextArgs(args), func(opt rune, value string) error {
		switch opt {
		case 'A':
			n, err := count(value, false)
			if err != nil {
				return err
			}
			opts.after = n
		case 'a':
			opts.binary = 'a'
		case 'B':
			n, err := count(value, false)
			if err != nil {
				return err
			}
			opts.before = n
		case 'b':
			opts.byteOffset = true
		case 'C':
			n, err := count(value, false)
			if err != nil {
				return err
			}
			opts.after, opts.before = n, n
		case 'c':
			opts.output = 'c'
		case 'D':
			switch value {
			case "read":
				opts.devices = 'r'
			case "skip":
				opts.devices = 's'
			default:
				return fmt.Errorf("invalid argument '%s' for '--devices'", value)
			}
		case 'd':
			switch value {
			case "read":
				opts.dirs = 0
			case "skip":
				opts.dirs = 's'
			case "recurse":
				opts.dirs = 'R'
				if opts.follow == 0 {
					opts.follow = 'H'
				}
			default:
				return fmt.Errorf("invalid argument '%s' for '--directories'", value)
			}
		case 'E':
			opts.mode = 'E'
		case 'e':
			opts.patternLists = append(opts.patternLists, value)
		case 'F':
			opts.mode = 'F'
		case 'f':
			opts.patternFiles = append(opts.patternFiles, value)
		case 'G':
			opts.mode = 'B'
		case 'H':
			opts.filename = 1
		case 'h':
			opts.filename = -1
		case 'I':
			opts.binary = 'I'
		case 'i', 'y':
			opts.ignoreCase = true
		case 'L':
			opts.output = 'L'
		case 'l':
			opts.output = 'l'
		case 'm':
			n, err := count(value, true)
			if err != nil {
				return err
			}
			opts.maxCount = n
		case 'n':
			opts.lineNum = true
		case 'o':
			opts.onlyMatching = true
		case 'q':
			opts.output = 'q'
		case 'R':
			opts.dirs = 'R'
			opts.follow = 'L'
		case 'r':
			opts.dirs = 'R'
			opts.follow = 'H'
		case 's':
			opts.suppress = true
		case 'T':
			opts.initialTab = true
		case 'v':
			opts.invert = true
		case 'w':
			opts.words = true
		case 'x':
			opts.whole = true
		case 'Z':
			opts.nullName = true
		case 'z':
			opts.nullData = true
		case optBinaryFiles:
			switch value {
			case "binary":
				opts.binary = 0
			case "without-match":
				opts.binary = 'I'
			case "text":
				opts.binary = 'a'
			default:
				return fmt.Errorf("invalid argument '%s' for '--binary-files'", value)
			}
		case optColor:
			if value == "" {
				// A plain --color means auto, like the option argument.
				value = "auto"
			}
			switch value {
			case "always":
				opts.color = 'y'
			case "never":
				opts.color = 'n'
			case "auto":
				opts.color = 'a'
			default:
				return fmt.Errorf("invalid argument '%s' for '--color'", value)
			}
		case optExclude:
			return opts.addSelector(value, false)
		case optExcludeDir:
			re, err := glob.Compile(strings.TrimRight(value, "/"), false)
			if err != nil {
				return err
			}
			opts.excludeDirs = append(opts.excludeDirs, re)
		case optExcludeFrom:
			opts.excludeFiles = append(opts.excludeFiles, value)
		case optGroupSep:
			opts.groupSep = value
		case optInclude:
			return opts.addSelector(value, true)
		case optLabel:
			opts.label = value
		case optLineBuffered, optMmap:
			// The output is already written line by line, and the input
			// read directly.
		case optNoGroupSep:
			opts.groupSepNo = true
		case optNoIgnoreCase:
			opts.ignoreCase = false
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}
