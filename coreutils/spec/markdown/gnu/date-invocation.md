# date invocation (GNU Coreutils 9.12)
### 21.1 `date`: Print or set system date and time

Synopses:

```
date [option]... [+format]
date [option]... mmddhhmm[[cc]yy][.ss]
```

The `date` command displays the date and time.
With the `--set` (`-s`) option, or with
‘`mmddhhmm[[cc]yy][.ss]`’,
it sets the date and time before displaying it.

Invoking `date` with no operands is equivalent to invoking
it with a default format that depends on the `LC_TIME` locale category.
In the default C locale, this format is ‘`+%a %b %e %H:%M:%S %Z %Y`’,
so the output looks like ‘`Thu Jul  9 17:00:00 EDT 2026`’.

Normally, `date` uses the time zone rules indicated by the
`TZ` environment variable, or the system default rules if `TZ`
is not set. See [Specifying the Time Zone with
`TZ`](https://www.gnu.org/software/libc/manual/html_node/TZ-Variable.html#TZ-Variable) in The GNU C Library Reference Manual.

An exit status of zero indicates success,
and a nonzero value indicates failure.

* [Specifying the format of `date` output](Date-format-specifiers.md)
* [Calendars](Calendars.md)
* [Setting the time](Setting-the-time.md)
* [Options for `date`](Options-for-date.md)
* [Examples of `date`](Examples-of-date.md)
