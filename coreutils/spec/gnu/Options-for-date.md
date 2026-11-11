# Options for date (GNU Coreutils 9.12)
#### 21.1.4 Options for `date`

The program accepts the following options. Also see [Common options](Common-options.md).
Except for `-u`, these options are all GNU extensions to POSIX.

All options that specify the date to display are mutually exclusive.
I.e.: `--date`, `--file`, `--reference`,
`--resolution`.

‘`-d datestr`’

‘`--date=datestr`’
:   Display the date and time specified in datestr instead of the
    current date and time. datestr can be in almost any common
    format. It can contain month names, time zones, ‘`am`’ and ‘`pm`’,
    ‘`yesterday`’, etc. For example, `--date="2020-07-21
    14:19:13.489392193 +0530"` specifies the instant of time that is
    489,392,193 nanoseconds after July 21, 2020 at 2:19:13 PM in a
    time zone that is 5 hours and 30 minutes east of UTC.
    The datestr must be in locale independent format. E.g., the
    ‘`LC_TIME=C`’ below is needed to print the correct date in many locales:

    ```
    date -d "$(LC_TIME=C date)"
    ```

    See [Date input formats](Date-input-formats.md).

‘`--debug`’
:   Annotate the parsed date, display the effective time zone, and warn about
    potential misuse.

‘`-f datefile`’

‘`--file=datefile`’
:   Parse each line in datefile as with `-d` and display the
    resulting date and time. If datefile is ‘`-`’, use standard
    input. This is useful when you have many dates to process, because the
    system overhead of starting up the `date` executable many times can
    be considerable.

‘`-I[timespec]`’

‘`--iso-8601[=timespec]`’
:   Display the date using an ISO 8601 format, ‘`%Y-%m-%d`’.

    The argument timespec specifies the number of additional
    terms of the time to include. It can be one of the following:

    ‘`auto`’
    :   Print just the date. This is the default if timespec is omitted.
        This is like the format `%Y-%m-%d`.

    ‘`hours`’
    :   Also print hours and time zone.
        This is like the format `%Y-%m-%dT%H%:z`.

    ‘`minutes`’
    :   Also print minutes.
        This is like the format `%Y-%m-%dT%H:%M%:z`.

    ‘`seconds`’
    :   Also print seconds.
        This is like the format `%Y-%m-%dT%H:%M:%S%:z`.

    ‘`ns`’
    :   Also print nanoseconds.
        This is like the format `%Y-%m-%dT%H:%M:%S,%N%:z`.

        GNU `date` uses a ‘`,`’ decimal point as preferred by ISO
        8601. You can override it by expressing the full format with a ‘`.`’
        decimal point explicitly, like `LC_ALL=C date
        +'%Y-%m-%dT%H:%M:%S.%N%:z'`.

    This format is always suitable as input
    for the `--date` (`-d`) and `--file`
    (`-f`) options, regardless of the current locale.

‘`-r file`’

‘`--reference=file`’
:   Display the date and time of the last modification of file,
    instead of the current date and time.

‘`--resolution`’
:   Display the timestamp resolution instead of the time.
    Current clock timestamps that are output by `date`
    are integer multiples of the timestamp resolution.
    With this option, the format defaults to ‘`%s.%N`’.
    For example, if the clock resolution is 1 millisecond,
    the output is:

    ```
    0.001000000
    ```

‘`-R`’

‘`--rfc-email`’
:   Display the date and time using the format ‘`%a, %d %b %Y %H:%M:%S
    %z`’, evaluated in the C locale so abbreviations are always in English.
    For example:

    ```
    Mon, 09 Jul 2020 17:00:00 -0400
    ```

    This format conforms to Internet RFCs
    [5322](https://datatracker.ietf.org/doc/rfc5322/),
    [2822](https://datatracker.ietf.org/doc/rfc2822/) and
    [822](https://datatracker.ietf.org/doc/rfc822/), the
    current and previous standards for Internet email.
    For compatibility with older versions of `date`,
    `--rfc-2822` and `--rfc-822` are aliases for
    `--rfc-email`.

‘`--rfc-3339=timespec`’
:   Display the date using a format specified by
    [Internet
    RFC 3339](https://datatracker.ietf.org/doc/rfc3339/). This is like `--iso-8601`, except that a space rather
    than a ‘`T`’ separates dates from times, and a period rather than
    a comma separates seconds from subseconds.
    This format is always suitable as input
    for the `--date` (`-d`) and `--file`
    (`-f`) options, regardless of the current locale.

    The argument timespec specifies how much of the time to include.
    It can be one of the following:

    ‘`date`’
    :   Print just the full-date, e.g., ‘`2020-07-21`’.
        This is like the format ‘`%Y-%m-%d`’.

    ‘`seconds`’
    :   Print the full-date and full-time separated by a space, e.g.,
        ‘`2020-07-21 04:30:37+05:30`’. The output ends with a numeric
        time-offset; here the ‘`+05:30`’ means that local time is five
        hours and thirty minutes east of UTC. This is like
        the format ‘`%Y-%m-%d %H:%M:%S%:z`’.

    ‘`ns`’
    :   Like ‘`seconds`’, but also print nanoseconds, e.g.,
        ‘`2020-07-21 04:30:37.998458565+05:30`’.
        This is like the format ‘`%Y-%m-%d %H:%M:%S.%N%:z`’.

‘`-s datestr`’

‘`--set=datestr`’
:   Set the date and time to datestr. See `-d` above.
    See also [Setting the time](Setting-the-time.md).

‘`-u`’

‘`--utc`’

‘`--universal`’
:   Use Universal Time by operating as if the
    `TZ` environment variable were set to the string ‘`UTC0`’.
    UTC stands for Coordinated Universal Time, established in 1960.
    Universal Time is often called “Greenwich Mean Time” (GMT) for
    historical reasons.
    Typically, systems ignore leap seconds and thus implement an
    approximation to UTC rather than true UTC.
