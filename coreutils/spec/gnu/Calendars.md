# Calendars (GNU Coreutils 9.12)
#### 21.1.2 Calendars

A calendar is a system that groups some sequences of days into “months”,
and some sequences of months into “years”.
In most countries, the calendar in use is the Gregorian calendar,
with months ranging from January (the first month) to December (the 12th month).

In some specific countries, other calendars are in effect, and
GNU `date` supports them in its output.
The calendar depends on the `LC_TIME` category of the current locale:

* In the locale of Thailand (`th_TH.UTF-8`),
  the Thai solar calendar is used.
* In the locale of Iran (`fa_IR`),
  the Persian solar Hijri calendar is used.
* In the locale of Ethiopia (`am_ET`),
  the Ethiopian / Ge’ez calendar is used.

The dates in these calendars look different.
For example, the day the GNU project was announced:

```
$ LC_ALL=en_US.UTF-8 date +%Y-%m-%d -d 1983-09-27
1983-09-27
$ LC_ALL=th_TH.UTF-8 date +%Y-%m-%d -d 1983-09-27
2526-09-27
$ LC_ALL=fa_IR date +%Y-%m-%d -d 1983-09-27
1362-07-05
$ LC_ALL=am_ET date +%Y-%m-%d -d 1983-09-27
1976-01-16
```

In these locales, when a non-Gregorian calendar is in use,
a few date conversion specifiers should not be used,
because they don’t apply in the expected way:

* ‘`%C`’ for the century,
* ‘`%y`’ for a two-digit year,
* ‘`%G`’ and ‘`%g`’ for a week-based year,
* ‘`%U`’, ‘`%W`’, and ‘`%V`’ for the week in the year,
* ‘`%j`’ for the day in the year,
* and ‘`%D`’ for the date in U.S. formatting convention.

If you are using one of these locales
and want to see Gregorian dates nevertheless,
you can achieve that by

1. making sure that you use the `LANG` environment variable,
   not the `LC_ALL` environment variable,
   for specifying your general locale, and
2. setting the `LC_TIME` environment variable to a locale
   that uses the Gregorian calendar.

For example:

```
$ unset LC_ALL
$ LANG=fa_IR LC_TIME=en_US.UTF-8 date +%Y-%m-%d -d 1983-09-27
1983-09-27
```

For more information on how to set your locale, see the GNU gettext manual at
<https://www.gnu.org/software/gettext/manual/html_node/Users.html>.
