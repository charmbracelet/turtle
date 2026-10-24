# Examples of date (GNU Coreutils 9.12)
#### 21.1.5 Examples of `date`

Here are a few examples. Also see the documentation for the `-d`
option in the previous section.

* To print the date of the day before yesterday:

  ```
  date --date='2 days ago'
  ```
* To print the date of the day three months and one day hence:

  ```
  date --date='3 months 1 day'
  ```
* To print the day of year of Christmas in the current year:

  ```
  date --date='25 Dec' +%j
  ```
* To print the current full month name and the day of the month:

  ```
  date '+%B %d'
  ```

  But this may not be what you want because for the first nine days of
  the month, the ‘`%d`’ expands to a zero-padded two-digit field,
  for example ‘`date -d 1may '+%B %d'`’ will print ‘`May 01`’.
* To print a date without the leading zero for one-digit days
  of the month, you can use the (GNU extension)
  ‘`-`’ flag to suppress
  the padding altogether:

  ```
  date -d 1may '+%B %-d'
  ```
* To print the current date and time in the format required by many
  non-GNU versions of `date` when setting the system clock:

  ```
  date +%m%d%H%M%Y.%S
  ```
* To set the system clock forward by two minutes:

  ```
  date --set='+2 minutes'
  ```
* To print the date in Internet RFC 5322 format,
  use ‘`date --rfc-email`’. Here is some example output:

  ```
  Tue, 09 Jul 2020 19:00:37 -0400
  ```
* To convert a date string to the number of seconds since the Epoch
  (which is 1970-01-01 00:00 UTC), use the `--date` option with
  the ‘`%s`’ format. That can be useful in sorting and/or graphing
  and/or comparing data by date. The following command outputs the
  number of the seconds since the Epoch for the time two minutes after the
  Epoch:

  ```
  date --date='1970-01-01 00:02:00 +0000' +%s
  120
  ```

  To convert a date string from one time zone from to another to,
  specify ‘`TZ="from"`’ in the environment and ‘`TZ="to"`’
  in the `--date` option. See [Specifying time zone rules](Specifying-time-zone-rules.md).
  For example:

  ```
  TZ="Asia/Tokyo" date --date='TZ="America/New_York" 2023-05-07 12:23'
  Mon May  8 01:23:00 JST 2023
  ```

  If you do not specify time zone information in the date string,
  `date` uses your computer’s idea of the time zone when
  interpreting the string. For example, if your computer’s time zone is
  that of Cambridge, Massachusetts, which was then 5 hours (i.e., 18,000
  seconds) behind UTC:

  ```
  # local time zone used
  date --date='1970-01-01 00:02:00' +%s
  18120
  ```
* If you’re sorting or graphing dated data, your raw date values may be
  represented as seconds since the Epoch. But few people can look at
  the date ‘`1577836800`’ and casually note “Oh, that’s the first
  second of the year 2020 in Greenwich, England.”

  ```
  date --date='2020-01-01 UTC' +%s
  1577836800
  ```

  An alternative is to use the `--utc` (`-u`) option.
  Then you may omit ‘`UTC`’ from the date string. Although this
  produces the same result for ‘`%s`’ and many other format sequences,
  with a time zone offset different from zero, it would give a different
  result for zone-dependent formats like ‘`%z`’.

  ```
  date -u --date=2020-07-21 +%s
  1595289600
  ```

  To convert such an unwieldy number of seconds back to
  a more readable form, use a command like this:

  ```
  date -d @1595289600 +"%F %T %z"
  2020-07-20 20:00:00 -0400
  ```

  Often it is better to output UTC-relative date and time:

  ```
  date -u -d @1595289600 +"%F %T %z"
  2020-07-21 00:00:00 +0000
  ```
* Typically the seconds count omits leap seconds, but some systems are
  exceptions. Because leap seconds are not predictable, the mapping
  between the seconds count and a future timestamp is not reliable on
  the atypical systems that include leap seconds in their counts.

  Here is how the two kinds of systems handle the leap second at
  the end of the year 2016:

  ```
  # Typical systems ignore leap seconds:
  date --date='2016-12-31 23:59:59 +0000' +%s
  1483228799
  date --date='2016-12-31 23:59:60 +0000' +%s
  date: invalid date '2016-12-31 23:59:60 +0000'
  date --date='2017-01-01 00:00:00 +0000' +%s
  1483228800
  ```

  ```
  # Atypical systems count leap seconds:
  date --date='2016-12-31 23:59:59 +0000' +%s
  1483228825
  date --date='2016-12-31 23:59:60 +0000' +%s
  1483228826
  date --date='2017-01-01 00:00:00 +0000' +%s
  1483228827
  ```
