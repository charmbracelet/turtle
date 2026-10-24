# Version-sort ordering rules (GNU Coreutils 9.12)
#### 30.2.1 Version-sort ordering rules

The version sort ordering rules are:

1. The strings are compared from left to right.
2. First the initial part of each string consisting entirely of non-digit
   bytes is determined.
   1. These two parts (either of which may be empty) are compared lexically.
      If a difference is found it is returned.
   2. The lexical comparison is a lexicographic comparison of byte strings,
      except that:
      1. ASCII letters sort before other bytes.
      2. A tilde sorts before anything, even an empty string.
3. Then the initial part of the remainder of each string that contains
   all the leading digits is determined. The numerical values represented by
   these two parts are compared, and any difference found is returned as
   the result of the comparison.
   1. For these purposes an empty string (which can only occur at the end of
      one or both version strings being compared) counts as zero.
   2. Because the numerical value is used, non-identical strings can compare
      equal. For example, ‘`123`’ compares equal to ‘`00123`’, and
      the empty string compares equal to ‘`0`’.
4. These two steps (comparing and removing initial non-digit strings and
   initial digit strings) are repeated until a difference is found or
   both strings are exhausted.

Consider the version-sort comparison of two file names:
`foo07.7z` and `foo7a.7z`. The two strings will be broken
down to the following parts, and the parts compared respectively from
each string:

```
foo  vs  foo   (rule 2, non-digits)
07   vs  7     (rule 3, digits)
.    vs  a.    (rule 2)
7    vs  7     (rule 3)
z    vs  z     (rule 2)
```

Comparison flow based on above algorithm:

1. The first parts (‘`foo`’) are identical.
2. The second parts (‘`07`’ and ‘`7`’) are compared numerically,
   and compare equal.
3. The third parts (‘`.`’ vs ‘`a.`’) are compared
   lexically by ASCII value (rule 2.B).
4. The first byte of the first string (‘`.`’) is compared
   to the first byte of the second string (‘`a`’).
5. Rule 2.B.a says letters sorts before non-letters.
   Hence, ‘`a`’ comes before ‘`.`’.
6. The returned result is that `foo7a.7z` comes before `foo07.7z`.

Result when using sort:

```
$ cat input3
foo07.7z
foo7a.7z
$ sort -V input3
foo7a.7z
foo07.7z
```

See [Differences from Debian version sort](Differences-from-Debian-version-sort.md) for
additional rules that extend the Debian algorithm in Coreutils.
