# Version sort is not the same as numeric sort (GNU Coreutils 9.12)
#### 30.2.2 Version sort is not the same as numeric sort

Consider the following text file:

```
$ cat input4
8.10
8.5
8.1
8.01
8.010
8.100
8.49

Numerical Sort:                   Version Sort:

$ sort -n input4                  $ sort -V input4
8.01                              8.01
8.010                             8.1
8.1                               8.5
8.10                              8.010
8.100                             8.10
8.49                              8.49
8.5                               8.100
```

Numeric sort (‘`sort -n`’) treats the entire string as a single numeric
value, and compares it to other values. For example, ‘`8.1`’, ‘`8.10`’ and
‘`8.100`’ are numerically equivalent, and are ordered together. Similarly,
‘`8.49`’ is numerically less than ‘`8.5`’, and appears before first.

Version sort (‘`sort -V`’) first breaks down the string into digit and
non-digit parts, and only then compares each part (see annotated
example in [Version-sort ordering rules](Version_002dsort-ordering-rules.md)).

Comparing the string ‘`8.1`’ to ‘`8.01`’, first the
‘`8`’s are compared (and are identical), then the
dots (‘`.`’) are compared and are identical, and lastly the
remaining digits are compared numerically (‘`1`’ and ‘`01`’) –
which are numerically equal. Hence, ‘`8.01`’ and ‘`8.1`’
are grouped together.

Similarly, comparing ‘`8.5`’ to ‘`8.49`’ – the ‘`8`’
and ‘`.`’ parts are identical, then the numeric values ‘`5`’ and
‘`49`’ are compared. The resulting ‘`5`’ appears before ‘`49`’.

This sorting order (where ‘`8.5`’ comes before ‘`8.49`’) is common when
assigning versions to computer programs (while perhaps not intuitive
or “natural” for people).
