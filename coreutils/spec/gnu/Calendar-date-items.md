# Calendar date items (GNU Coreutils 9.12)
### 29.2 Calendar date items

A *calendar date item* specifies a day of the year. It is
specified differently, depending on whether the month is specified
numerically or literally. All these strings specify the same calendar date:

```
2022-11-14     # ISO 8601.
22-11-14       # Assume 19xx for 69 through 99,
               # 20xx for 00 through 68 (not recommended).
11/14/2022     # Common U.S. writing.
14.11.2022     # Common European writing.
14 November 2022
14 Nov 2022    # Three-letter abbreviations always allowed.
November 14, 2022
14-nov-2022
14nov2022
```

The year can also be omitted. In this case, the last specified year is
used, or the current year if none. For example:

```
11/14
14.11.
nov 14
```

Here are the rules.

For numeric months, the ISO 8601 format
‘`year-month-day`’ is allowed, where year is
any positive number, month is a number between 01 and 12, and
day is a number between 01 and 31. A leading zero must be present
if a number is less than ten. If year is 68 or smaller, then 2000
is added to it; otherwise, if year is less than 100,
then 1900 is added to it. The construct
‘`month/day/year`’, popular in the United States,
is accepted. Also ‘`month/day`’, omitting the year.
Similarly, the construct ‘`day.month.year`’, popular in
Europe, and its short form ‘`day.month.`’ are accepted.

Literal months may be spelled out in full: ‘`January`’,
‘`February`’, ‘`March`’, ‘`April`’, ‘`May`’, ‘`June`’,
‘`July`’, ‘`August`’, ‘`September`’, ‘`October`’,
‘`November`’ or ‘`December`’. Literal months may be abbreviated
to their first three letters, possibly followed by an abbreviating dot.
It is also permitted to write ‘`Sept`’ instead of ‘`September`’.

When months are written literally, the calendar date may be given as any
of the following:

```
day month year
day month
month day year
day-month-year
```

Or, omitting the year:

```
month day
```
