# The tilde ~ (GNU Coreutils 9.12)
#### 30.2.5 The tilde ‘`~`’

Rule 2.B.b says the tilde ‘`~`’ (ASCII 126) sorts
before other bytes, and before an empty string.

```
$ cat input7
1
1%
1.2
1~
~
$ sort -V input7
~
1~
1
1%
1.2
```

The sorting algorithm starts by breaking down the string into
non-digit (rule 2) and digit parts (rule 3).

In the above input file, only the last line in the input file starts
with a non-digit (‘`~`’). This is the first part. All other lines
in the input file start with a digit – their first non-digit part is
empty.

Based on rule 2.B.b, tilde ‘`~`’ sorts before other bytes
and before the empty string – hence it comes before all other strings,
and is listed first in the sorted output.

The remaining lines (‘`1`’, ‘`1%`’, ‘`1.2`’, ‘`1~`’)
follow similar logic: The digit part is extracted (1 for all strings)
and compares equal. The following extracted parts for the remaining
input lines are: empty part, ‘`%`’, ‘`.`’, ‘`~`’.

Tilde sorts before all others, hence the line ‘`1~`’ appears next.

The remaining lines (‘`1`’, ‘`1%`’, ‘`1.2`’) are sorted based
on previously explained rules.
