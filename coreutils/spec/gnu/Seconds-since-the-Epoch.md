# Seconds since the Epoch (GNU Coreutils 9.12)
### 29.9 Seconds since the Epoch

If you precede a number with ‘`@`’, it represents an internal
timestamp as a count of seconds. The number can contain an internal
decimal point (either ‘`.`’ or ‘`,`’); any excess precision not
supported by the internal representation is truncated toward minus
infinity. Such a number cannot be combined with any other date
item, as it specifies a complete timestamp.

Internally, computer times are represented as a count of seconds since
an Epoch—a well-defined point of time. On GNU and
POSIX systems, the Epoch is 1970-01-01 00:00:00 UTC, so
‘`@0`’ represents this time, ‘`@1`’ represents 1970-01-01
00:00:01 UTC, and so forth. GNU and most other
POSIX-compliant systems support such times as an extension
to POSIX, using negative counts, so that ‘`@-1`’
represents 1969-12-31 23:59:59 UTC.

Most modern systems count seconds with 64-bit two’s-complement integers
of seconds with nanosecond subcounts, which is a range that includes
the known lifetime of the universe with nanosecond resolution.
Some obsolescent systems count seconds with 32-bit two’s-complement
integers and can represent times from 1901-12-13 20:45:52 through
2038-01-19 03:14:07 UTC. A few systems sport other time ranges.

On most hosts, these counts ignore the presence of leap seconds.
For example, on most hosts ‘`@1483228799`’ represents 2016-12-31
23:59:59 UTC, ‘`@1483228800`’ represents 2017-01-01 00:00:00
UTC, and there is no way to represent the intervening leap second
2016-12-31 23:59:60 UTC.
