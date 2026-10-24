# Connectives for test (GNU Coreutils 9.12)
#### 16.3.6 Connectives for `test`

It is better to use shell logical primitives
rather than these logical connectives internal to `test`,
because an expression may become ambiguous
depending on the expansion of its parameters.

For example, this becomes ambiguous when ‘`$1`’
is set to ‘`'!'`’ and ‘`$2`’ to the empty string ‘`''`’:

```
test "$1" -a "$2"
```

and should be written as:

```
test "$1" && test "$2"
```

The shell logical primitives also benefit from
short circuit operation, which can be significant
for file attribute tests.

‘`! expr`’
:   True if expr is false.
    ‘`!`’ has lower precedence than all parts of expr.
    The ‘`!`’ should be specified to the left
    of a binary expression, I.e., ‘`! 1 -gt 2`’
    rather than ‘`1 ! -gt 2`’.

‘`expr1 -a expr2`’
:   True if both expr1 and expr2 are true.
    ‘`-a`’ is left associative,
    and has a higher precedence than ‘`-o`’.

‘`expr1 -o expr2`’
:   True if either expr1 or expr2 is true.
    ‘`-o`’ is left associative.
