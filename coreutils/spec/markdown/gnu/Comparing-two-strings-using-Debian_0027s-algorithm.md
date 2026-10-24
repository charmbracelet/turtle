# Comparing two strings using Debian's algorithm (GNU Coreutils 9.12)
#### 30.3.4 Comparing two strings using Debian’s algorithm

The Debian program `dpkg` (available on all Debian and Ubuntu
installations) can compare two strings using the `--compare-versions`
option.

To use it, create a helper shell function (simply copy & paste the
following snippet to your shell command-prompt):

```
compver() {
  if dpkg --compare-versions "$1" lt "$2"
  then printf '%s\n' "$1" "$2"
  else printf '%s\n' "$2" "$1"
  fi
}
```

Then compare two strings by calling `compver`:

```
$ compver 8.49 8.5
8.5
8.49
```

Note that `dpkg` will warn if the strings have invalid syntax:

```
$ compver "foo07.7z" "foo7a.7z"
dpkg: warning: version 'foo07.7z' has bad syntax:
               version number does not start with digit
dpkg: warning: version 'foo7a.7z' has bad syntax:
               version number does not start with digit
foo7a.7z
foo07.7z
$ compver "3.0/" "3.0.5"
dpkg: warning: version '3.0/' has bad syntax:
               invalid character in version number
3.0.5
3.0/
```

To illustrate the different handling of hyphens between Debian and
Coreutils algorithms (see
[Hyphen-minus ‘`-`’ and colon ‘`:`’](Hyphen_002dminus-and-colon.md)):

```
$ compver abb ab-cd 2>/dev/null     $ printf 'abb\nab-cd\n' | sort -V
ab-cd                               abb
abb                                 ab-cd
```

To illustrate the different handling of file extension: (see [Special handling of file extensions](Special-handling-of-file-extensions.md)):

```
$ compver hello-8.txt hello-8.2.txt 2>/dev/null
hello-8.2.txt
hello-8.txt
$ printf '%s\n' hello-8.txt hello-8.2.txt | sort -V
hello-8.txt
hello-8.2.txt
```
