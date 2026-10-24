# true
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

true — return true value

#### SYNOPSIS

`true`

#### DESCRIPTION

The *true* utility shall return with exit code zero.

#### OPTIONS

None.

#### OPERANDS

None.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

None.

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

Not used.

#### STDERR

Not used.

#### OUTPUT FILES

None.

#### EXTENDED DESCRIPTION

None.

#### EXIT STATUS

Zero.

#### CONSEQUENCES OF ERRORS

None.

*The following sections are informative.*

#### APPLICATION USAGE

This utility is typically used in shell scripts, as shown in the EXAMPLES section.

Although the special built-in utility **:** ([*colon*](../utilities/colon.html)) is similar to *true*,
there are some notable differences, including:

* Whereas [*colon*](../utilities/colon.html) is required to accept, and do nothing with, any number of arguments,
  *true* is only required to accept, and discard, a first argument of "--". Passing any other argument(s) to
  *true* may cause its behavior to differ from that described in this standard.
* A non-interactive shell exits when a redirection error occurs with [*colon*](../utilities/colon.html) (unless
  executed via [*command*](../utilities/command.html)), whereas with *true* it does not.
* Variable assignments preceding the command name persist after executing [*colon*](../utilities/colon.html)
  (unless executed via [*command*](../utilities/command.html)), but not after executing *true*.
* In shell implementations where *true* is not provided as a built-in, using [*colon*](../utilities/colon.html) avoids the overheads associated with executing an external utility.

#### EXAMPLES

This command is executed forever:

```

while true
do
    command
done
```

#### RATIONALE

None.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*2.9 Shell Commands*](../utilities/V3_chap02.html#tag_19_09), [*colon*](../utilities/V3_chap02.html#tag_19_17), [*command*](../utilities/command.html#), [*false*](../utilities/false.html#)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 6

IEEE Std 1003.1-2001/Cor 1-2002, item XCU/TC1/D6/39 is applied, replacing the terms "None" and "Default" from
the STDERR and EXIT STATUS sections, respectively, with terms as defined in [*1.4
Utility Description Defaults*](../utilities/V3_chap01.html#tag_18_04).

#### Issue 8

Austin Group Defect 1640 is applied, clarifying the differences between *true* and **:** ([*colon*](../utilities/colon.html)).

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
