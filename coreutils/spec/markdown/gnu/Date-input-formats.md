# Date input formats (GNU Coreutils 9.12)
## 29 Date input formats

First, a quote:

> Our units of temporal measurement, from seconds on up to months, are so
> complicated, asymmetrical and disjunctive so as to make coherent mental
> reckoning in time all but impossible. Indeed, had some tyrannical god
> contrived to enslave our minds to time, to make it all but impossible
> for us to escape subjection to sodden routines and unpleasant surprises,
> he could hardly have done better than handing down our present system.
> It is like a set of trapezoidal building blocks, with no vertical or
> horizontal surfaces, like a language in which the simplest thought
> demands ornate constructions, useless particles and lengthy
> circumlocutions. Unlike the more successful patterns of language and
> science, which enable us to face experience boldly or at least
> level-headedly, our system of temporal calculation silently and
> persistently encourages our terror of time.
>
> … It is as though architects had to measure length in feet, width
> in meters and height in ells; as though basic instruction manuals
> demanded a knowledge of five different languages. It is no wonder then
> that we often look into our own immediate past or future, last Tuesday
> or a week from Sunday, with feelings of helpless confusion. …
>
> —Robert Grudin, Time and the Art of Living.

This section describes the textual date representations that GNU
programs accept. These are the strings you, as a user, can supply as
arguments to the various programs. The C interface (via the
`parse_datetime` function) is not described here.

* [General date syntax](General-date-syntax.md)
* [Calendar date items](Calendar-date-items.md)
* [Time of day items](Time-of-day-items.md)
* [Time zone items](Time-zone-items.md)
* [Combined date and time of day items](Combined-date-and-time-of-day-items.md)
* [Day of week items](Day-of-week-items.md)
* [Relative items in date strings](Relative-items-in-date-strings.md)
* [Pure numbers in date strings](Pure-numbers-in-date-strings.md)
* [Seconds since the Epoch](Seconds-since-the-Epoch.md)
* [Specifying time zone rules](Specifying-time-zone-rules.md)
* [Authors of `parse_datetime`](Authors-of-parse_005fdatetime.md)
