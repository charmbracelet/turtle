# test invocation (GNU Coreutils 9.12)
### 16.3 `test`: Check file types and compare values

`test` returns a status of 0 (true) or 1 (false) depending on the
evaluation of the conditional expression expr. Each part of the
expression must be a separate argument.

`test` has file status checks, string operators, and numeric
comparison operators.

`test` has an alternate form that uses opening and closing
square brackets instead a leading ‘`test`’. For example, instead
of ‘`test -d /`’, you can write ‘`[ -d / ]`’. The square
brackets must be separate arguments; for example, ‘`[-d /]`’ does
not have the desired effect. Since ‘`test expr`’ and ‘`[
expr ]`’ have the same meaning, only the former form is discussed
below.

Synopses:

```
test expression
test
[ expression ]
[ ]
[ option
```

Due to shell aliases and built-in `test` functions, using an
unadorned `test` interactively or in a script may get you
different functionality than that described here. Invoke it via
`env` (i.e., `env test …`) to avoid interference
from the shell.

If expression is omitted, `test` returns false.
If expression is a single argument,
`test` returns false if the argument is null and true
otherwise. The argument
can be any string, including strings like ‘`-d`’, ‘`-1`’,
‘`--`’, ‘`--help`’, and ‘`--version`’ that most other
programs would treat as options. To get help and version information,
invoke the commands ‘`[ --help`’ and ‘`[ --version`’, without
the usual closing brackets. See [Common options](Common-options.md).

Exit status:

```
0 if the expression is true,
1 if the expression is false,
2 if an error occurred.
```

* [File type tests](File-type-tests.md)
* [Access permission tests](Access-permission-tests.md)
* [File characteristic tests](File-characteristic-tests.md)
* [String tests](String-tests.md)
* [Numeric tests](Numeric-tests.md)
* [Connectives for `test`](Connectives-for-test.md)
