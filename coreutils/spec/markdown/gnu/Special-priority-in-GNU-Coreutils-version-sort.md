# Special priority in GNU Coreutils version sort (GNU Coreutils 9.12)
#### 30.3.2 Special priority in GNU Coreutils version sort

In GNU Coreutils version sort, the following items have
special priority and sort before all other strings (listed in order):

1. The empty string
2. The string ‘`.`’ (a single dot, ASCII 46)
3. The string ‘`..`’ (two dots)
4. Strings starting with dot (‘`.`’) sort before
   strings starting with any other byte.

Example:

```
$ printf '%s\n' a "" b "." c  ".."  ".d20" ".d3"  | sort -V
.
..
.d3
.d20
a
b
c
```

These priorities make perfect sense for ‘`ls -v`’: The special
files dot ‘`.`’ and dot-dot ‘`..`’ will be listed
first, followed by any hidden files (files starting with a dot),
followed by non-hidden files.

For ‘`sort -V`’ these priorities might seem arbitrary. However,
because the sorting code is shared between the `ls` and `sort`
program, the ordering rules are the same.
