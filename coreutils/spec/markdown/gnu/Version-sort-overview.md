# Version sort overview (GNU Coreutils 9.12)
### 30.1 Version sort overview

*Version sort* puts items such as file names and lines of
text in an order that feels natural to people, when the text
contains a mixture of letters and digits.

Lexicographic sorting usually does not produce the order that one expects
because comparisons are made on a character-by-character basis.

Compare the sorting of the following items:

```
Lexicographic sort:          Version Sort:

a1                           a1
a120                         a2
a13                          a13
a2                           a120
```

Version sort functionality in GNU Coreutils is available in the ‘`ls -v`’,
‘`ls --sort=version`’, ‘`sort -V`’, and
‘`sort --version-sort`’ commands.

* [Using version sort in GNU Coreutils](Using-version-sort-in-GNU-Coreutils.md)
* [Version sort and natural sort](Version-sort-and-natural-sort.md)
* [Variations in version sort order](Variations-in-version-sort-order.md)
