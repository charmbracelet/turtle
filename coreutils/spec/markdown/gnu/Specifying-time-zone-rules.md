# Specifying time zone rules (GNU Coreutils 9.12)
### 29.10 Specifying time zone rules

Normally, dates are interpreted using the rules of the current time
zone, which in turn are specified by the `TZ` environment
variable, or by a system default if `TZ` is not set. To specify a
different set of default time zone rules that apply just to one date,
start the date with a string of the form ‘`TZ="rule"`’. The
two quote characters (‘`"`’) must be present in the date, and any
quotes or backslashes within rule must be escaped by a
backslash.

For example, with the GNU `date` command you can
answer the question “What time is it in New York when a Paris clock
shows 6:30am on October 31, 2022?” by using a date beginning with
‘`TZ="Europe/Paris"`’ as shown in the following shell transcript:

```
$ export TZ="America/New_York"
$ date --date='TZ="Europe/Paris" 2022-10-31 06:30'
Mon Oct 31 01:30:00 EDT 2022
```

In this example, the `--date` operand begins with its own
`TZ` setting, so the rest of that operand is processed according
to ‘`Europe/Paris`’ rules, treating the string ‘`2022-11-14
06:30`’ as if it were in Paris. However, since the output of the
`date` command is processed according to the overall time zone
rules, it uses New York time. (Paris was normally six hours ahead of
New York in 2022, but this example refers to a brief Halloween period
when the gap was five hours.)

A `TZ` value is a rule that typically names a location in the
[‘`tz`’ database](https://www.iana.org/time-zones).
A recent catalog of location names appears in the
[TWiki Date and Time
Gateway](https://twiki.org/cgi-bin/xtra/tzdatepick.html). A few non-GNU hosts require a colon before a
location name in a `TZ` setting, e.g.,
‘`TZ=":America/New_York"`’.

The ‘`tz`’ database includes a wide variety of locations ranging
from ‘`Africa/Abidjan`’ to ‘`Pacific/Tongatapu`’, but
if you are at sea and have your own private time zone, or if you are
using a non-GNU host that does not support the ‘`tz`’
database, you may need to use a POSIX rule instead.
The previously-mentioned POSIX rule ‘`UTC0`’ says that the time zone
abbreviation is ‘`UTC`’, the zone is zero hours away from
Greenwich, and there is no daylight saving time.
POSIX rules can also specify nonzero Greenwich offsets.
For example, the following shell transcript answers the question
“What time is it five and a half hours east of Greenwich when a clock
seven hours west of Greenwich shows 9:50pm on July 12, 2022?”

```
$ TZ="<+0530>-5:30" date --date='TZ="<-07>+7" 2022-07-12 21:50'
Wed Jul 13 10:20:00 +0530 2022
```

This example uses the somewhat-confusing POSIX convention for rules.
‘`TZ="<-07>+7"`’ says that the time zone abbreviation is ‘`-07`’
and the time zone is 7 hours west of Greenwich, and
‘`TZ="<+0530>-5:30"`’ says that the time zone abbreviation is ‘`+0530`’
and the time zone is 5 hours 30 minutes east of Greenwich.
(One should never use a setting like ‘`TZ="UTC-5"`’, since
this would incorrectly imply that local time is five hours east of
Greenwich and the time zone is called “UTC”.)
Although trickier POSIX `TZ` settings like
‘`TZ="<-05>+5<-04>,M3.2.0/2,M11.1.0/2"`’ can specify some daylight
saving regimes, location-based settings like
‘`TZ="America/New_York"`’ are typically simpler and more accurate
historically. See [Specifying the Time Zone with `TZ`](https://www.gnu.org/software/libc/manual/html_node/TZ-Variable.html#TZ-Variable) in The GNU C Library.
