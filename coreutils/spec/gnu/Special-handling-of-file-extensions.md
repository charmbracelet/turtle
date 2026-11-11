# Special handling of file extensions (GNU Coreutils 9.12)
#### 30.3.3 Special handling of file extensions

GNU Coreutils version sort implements specialized handling
of strings that look like file names with extensions.
This enables slightly more natural ordering of file
names.

The following additional rules apply when comparing two strings where
both begin with non-‘`.`’. They also apply when comparing two
strings where both begin with ‘`.`’ but neither is ‘`.`’ or ‘`..`’.

1. A suffix (i.e., a file extension) is defined as: a dot, followed by an
   ASCII letter or tilde, followed by zero or more ASCII letters, digits,
   or tildes; all repeated zero or more times, and ending at string end.
   This is equivalent to matching the extended regular expression
   `(\.[A-Za-z~][A-Za-z0-9~]*)*$` in the C locale.
   The longest such match is used, except that a suffix is not
   allowed to match an entire nonempty string.
2. The suffixes are temporarily removed, and the strings are compared
   without them, using version sort (see [Version-sort ordering rules](Version_002dsort-ordering-rules.md)) without special priority (see [Special priority in GNU Coreutils version sort](Special-priority-in-GNU-Coreutils-version-sort.md)).
3. If the suffix-less strings do not compare equal, this comparison
   result is used and the suffixes are effectively ignored.
4. If the suffix-less strings compare equal, the suffixes are restored
   and the entire strings are compared using version sort.

Examples for rule 1:

* ‘`hello-8.txt`’: the suffix is ‘`.txt`’
* ‘`hello-8.2.txt`’: the suffix is ‘`.txt`’
  (‘`.2`’ is not included because the dot is not followed by a letter)
* ‘`hello-8.0.12.tar.gz`’: the suffix is ‘`.tar.gz`’ (‘`.0.12`’
  is not included)
* ‘`hello-8.2`’: no suffix (suffix is an empty string)
* ‘`hello.foobar65`’: the suffix is ‘`.foobar65`’
* ‘`gcc-c++-10.8.12-0.7rc2.fc9.tar.bz2`’: the suffix is
  ‘`.fc9.tar.bz2`’ (‘`.7rc2`’ is not included as it begins with a digit)
* ‘`.autom4te.cfg`’: the suffix is the entire string.

Examples for rule 2:

* Comparing ‘`hello-8.txt`’ to ‘`hello-8.2.12.txt`’, the
  ‘`.txt`’ suffix is temporarily removed from both strings.
* Comparing ‘`foo-10.3.tar.gz`’ to ‘`foo-10.tar.xz`’, the suffixes
  ‘`.tar.gz`’ and ‘`.tar.xz`’ are temporarily removed from the
  strings.

Example for rule 3:

* Comparing ‘`hello.foobar65`’ to ‘`hello.foobar4`’, the suffixes
  (‘`.foobar65`’ and ‘`.foobar4`’) are temporarily removed. The
  remaining strings are identical (‘`hello`’). The suffixes are then
  restored, and the entire strings are compared (‘`hello.foobar4`’ comes
  first).

Examples for rule 4:

* When comparing the strings ‘`hello-8.2.txt`’ and ‘`hello-8.10.txt`’, the
  suffixes (‘`.txt`’) are temporarily removed. The remaining strings
  (‘`hello-8.2`’ and ‘`hello-8.10`’) are compared as previously described
  (‘`hello-8.2`’ comes first).
  *(In this case the suffix removal algorithm
  does not have a noticeable effect on the resulting order.)*

**How does the suffix-removal algorithm effect ordering results?**

Consider the comparison of hello-8.txt and hello-8.2.txt.

Without the suffix-removal algorithm, the strings will be broken down
to the following parts:

```
hello-  vs  hello-  (rule 2, all non-digits)
8       vs  8       (rule 3, all digits)
.txt    vs  .       (rule 2)
empty   vs  2
empty   vs  .txt
```

The comparison of the third parts (‘`.`’ vs
‘`.txt`’) will determine that the shorter string comes first –
resulting in `hello-8.2.txt` appearing first.

Indeed this is the order in which Debian’s `dpkg` compares the strings.

A more natural result is that `hello-8.txt` should come before
`hello-8.2.txt`, and this is where the suffix-removal comes into play:

The suffixes (‘`.txt`’) are removed, and the remaining strings are
broken down into the following parts:

```
hello-  vs  hello-  (rule 2, all non-digits)
8       vs  8       (rule 3, all digits)
empty   vs  .       (rule 2)
empty   vs  2
```

As empty strings sort before non-empty strings, the result is ‘`hello-8`’
being first.

A real-world example would be listing files such as:
`gcc_10.fc9.tar.gz`
and `gcc_10.8.12.7rc2.fc9.tar.bz2`: Debian’s algorithm would list
`gcc_10.8.12.7rc2.fc9.tar.bz2` first, while ‘`ls -v`’ will list
`gcc_10.fc9.tar.gz` first.

These priorities make sense for ‘`ls -v`’:
Versioned files will be listed in a more natural order.

For ‘`sort -V`’ these priorities might seem arbitrary. However,
because the sorting code is shared between the `ls` and `sort`
program, the ordering rules are the same.
