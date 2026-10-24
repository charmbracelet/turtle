# Setting the time (GNU Coreutils 9.12)
#### 21.1.3 Setting the time

You must have appropriate privileges to set the
system clock. For changes to persist across a reboot, the
hardware clock may need to be updated from the system clock, which
might not happen automatically on your system.

To set the clock, you can use the `--set` (`-s`) option
(see [Options for `date`](Options-for-date.md)) or a operand of the form
‘`mmddhhmm[[cc]yy][.ss]`’
where each two-letter
component stands for two digits with the following meanings:

mm
:   month (this is the first mm)

dd
:   day within month

hh
:   hour

mm
:   minute (this is the second mm)

cc
:   first two digits of year (optional)

yy
:   last two digits of year (optional)

ss
:   second (optional; this is a GNU extension)

The `--date` and `--set` options may not be used with an
argument in the above format. The `--universal` option may be used
with such an argument to indicate that the specified date and time are
relative to Universal Time rather than to the local time zone.
