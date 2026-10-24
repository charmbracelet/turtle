# General date syntax (GNU Coreutils 9.12)
### 29.1 General date syntax

A *date* is a string, possibly empty, containing many items
separated by whitespace. The whitespace may be omitted when no
ambiguity arises. The empty string means the beginning of today (i.e.,
midnight). Order of the items is immaterial. A date string may contain
many flavors of items:

* calendar date items
* time of day items
* time zone items
* combined date and time of day items
* day of the week items
* relative items
* pure numbers.

We describe each of these item types in turn, below.

A few ordinal numbers may be written out in words in some contexts. This is
most useful for specifying day of the week items or relative items (see
below). Among the most commonly used ordinal numbers, the word
‘`last`’ stands for *-1*, ‘`this`’ stands for 0, and
‘`first`’ and ‘`next`’ both stand for 1. Because the word
‘`second`’ stands for the unit of time there is no way to write the
ordinal number 2, but for convenience ‘`third`’ stands for 3,
‘`fourth`’ for 4, ‘`fifth`’ for 5,
‘`sixth`’ for 6, ‘`seventh`’ for 7, ‘`eighth`’ for 8,
‘`ninth`’ for 9, ‘`tenth`’ for 10, ‘`eleventh`’ for 11 and
‘`twelfth`’ for 12.

When a month is written this way, it is still considered to be written
numerically, instead of being “spelled in full”; this changes the
allowed strings.

In the current implementation, only English is supported for words and
abbreviations like ‘`AM`’, ‘`DST`’, ‘`EST`’, ‘`first`’,
‘`January`’, ‘`Sunday`’, ‘`tomorrow`’, and ‘`year`’.

In the current implementation, only the Gregorian calendar is supported,
regardless of the locale.

The output of the `date` command
is not always acceptable as a date string,
not only because of the language problem and of the calendar problem,
but also because there is no
standard meaning for time zone items like ‘`IST`’. When using
`date` to generate a date string intended to be parsed later,
specify a date format that is independent of language,
that uses the Gregorian calendar,
and that does not use time zone items other than ‘`UTC`’ and ‘`Z`’.
Here are some ways to do this:

```
$ LC_ALL=C TZ=UTC0 date
Tue Nov 15 02:02:42 UTC 2022
$ LC_ALL=C TZ=UTC0 date +'%Y-%m-%d %H:%M:%SZ'
2022-11-15 02:02:42Z
$ date --rfc-3339=ns  # --rfc-3339 is a GNU extension.
2022-11-14 21:02:42.000000000-05:00
$ date --rfc-email  # a GNU extension
Mon, 14 Nov 2022 21:02:42 -0500
$ LC_ALL=C date +'%Y-%m-%d %H:%M:%S %z'  # %z is a GNU extension.
2022-11-14 21:02:42 -0500
$ date +'@%s.%N'  # %s and %N are GNU extensions.
@1668477762.692722128
```

Alphabetic case is completely ignored in dates. Comments may be introduced
between round parentheses, as long as included parentheses are properly
nested. Hyphens not followed by a digit are currently ignored. Leading
zeros on numbers are ignored.

Invalid dates like ‘`2022-02-29`’ or times like ‘`24:00`’ are
rejected. In the typical case of a host that does not support leap
seconds, a time like ‘`23:59:60`’ is rejected even if it
corresponds to a valid leap second.
