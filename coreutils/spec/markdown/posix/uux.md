# uux
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

uux — remote command execution

#### SYNOPSIS

`[UU] ![[Option Start]](../images/opt-start.gif) uux [-jnp] command-string ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

The *uux* utility shall gather zero or more files from various systems, execute a shell pipeline (see [*2.9 Shell Commands*](../utilities/V3_chap02.html#tag_19_09)) on a specified system, and then send the standard output of
the command to a file on a specified system. Only the first command of a pipeline can have a *system-name*! prefix. All other
commands in the pipeline shall be executed on the system of the first command.

The following restrictions are applicable to the shell pipeline processed by *uux*:

* In gathering files from different systems, pathname expansion shall not be performed by *uux*. Thus, a request such as:

  ```

  uux "c17 remsys!~/*.c"
  ```

  would attempt to copy the file named literally **\*.c** to the local system.
* The redirection operators ">>", "<<", ">|", and ">&" shall not be
  accepted. Any use of these redirection operators shall cause this utility to write an error message describing the problem and exit
  with a non-zero exit status.
* The reserved word **!** cannot be used at the head of the pipeline to modify the exit status. (See the *command-string*
  operand description below.)
* Alias substitution shall not be performed.

A filename can be specified as for [*uucp*](../utilities/uucp.html); it can be an absolute pathname, a pathname
preceded by ~*name* (which is replaced by the corresponding login directory), a pathname specified as ~/*dest*
(*dest* is prefixed by the public directory called *PUBDIR*; the actual location of *PUBDIR* is
implementation-defined), or a simple filename (which is prefixed by *uux* with the current directory). See [*uucp*](../utilities/uucp.html#) for the details.

The execution of commands on remote systems shall take place in an execution directory known to the [*uucp*](../utilities/uucp.html) system. All files required for the execution shall be put into this directory unless they
already reside on that machine. Therefore, the application shall ensure that non-local filenames (without path or machine
reference) are unique within the *uux* request.

The *uux* utility shall attempt to get all files to the execution system. For files that are output files, the application
shall ensure that the filename is escaped using parentheses.

The remote system shall notify the user by mail if the requested command on the remote system was disallowed or the files were
not accessible. This notification can be turned off by the **-n** option.

Typical implementations of this utility require a communications line configured to use XBD [*11. General Terminal Interface*](../basedefs/V1_chap11.html#tag_11), but other communications means may be used. On
systems where there are no available communications means (either temporarily or permanently), this utility shall write an error
message describing the problem and exit with a non-zero exit status.

The *uux* utility cannot guarantee support for all character encodings in all circumstances. For example, transmission data
may be restricted to 7 bits by the underlying network, 8-bit data and filenames need not be portable to non-internationalized
systems, and so on. Under these circumstances, it is recommended that only characters defined in the ISO/IEC 646:1991 standard
International Reference Version (equivalent to ASCII) 7-bit range of characters be used and that only characters defined in the
portable filename character set be used for naming files.

#### OPTIONS

The *uux* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

The following options shall be supported:

**-j**
:   Write the job identification string to standard output. This job identification can be used by [*uustat*](../utilities/uustat.html) to obtain the status or terminate a job.

**-n**
:   Do not notify the user if the command fails.

**-p**
:   Make the standard input to *uux* the standard input to the *command-string*.

#### OPERANDS

The following operand shall be supported:

*command-string*
:   A string made up of one or more arguments that are similar to normal command arguments, except that the command and any filenames
    can be prefixed by *system-name*!. A null *system-name* shall be interpreted as the local system.

#### STDIN

The standard input shall not be used unless the '-' or **-p** option is specified; in those cases, the standard
input shall be made the standard input of the *command-string*.

#### INPUT FILES

Input files shall be selected according to the contents of *command-string*.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *uux*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) for the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as
    opposed to multi-byte characters in arguments).

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written to standard error.

*NLSPATH*
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the location of messages objects and message catalogs. ![[Option End]](../images/opt-end.gif)

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

The standard output shall not be used unless the **-j** option is specified; in that case, the job identification string
shall be written to standard output in the following format:

```

"%s\n", <jobid>
```

#### STDERR

The standard error shall be used only for diagnostic messages.

#### OUTPUT FILES

Output files shall be created or written, or both, according to the contents of *command-string*.

If **-n** is not used, mail files shall be modified following any command or file-access failures on the remote system.

#### EXTENDED DESCRIPTION

None.

#### EXIT STATUS

The following exit values shall be returned:

0
:   Successful completion.

>0
:   An error occurred.

#### CONSEQUENCES OF ERRORS

Default.

*The following sections are informative.*

#### APPLICATION USAGE

This utility is part of the UUCP Utilities option and need not be supported by all implementations.

Note that, for security reasons, many installations limit the list of commands executable on behalf of an incoming request from
*uux*. Many sites permit little more than the receipt of mail via *uux*.

Any characters special to the command interpreter should be quoted either by quoting the entire *command-string* or quoting
the special characters as individual arguments.

As noted in [*uucp*](../utilities/uucp.html), shell pattern matching notation characters appearing in pathnames
are expanded on the appropriate local system. This is done under the control of local settings of *LC_COLLATE* and *LC_CTYPE
.* Thus, care should be taken when using bracketed filename patterns, as collation and typing rules may vary from one system to
another. Also be aware that certain types of expression (that is, equivalence classes, character classes, and collating symbols)
need not be supported on non-internationalized systems.

#### EXAMPLES

1. The following command gets **file1** from system **a** and **file2** from system **b**, executes [*diff*](../utilities/diff.html) on the local system, and puts the results in **file.diff** in the local *PUBDIR*
   directory. (*PUBDIR* is the [*uucp*](../utilities/uucp.html) public directory on the local system.)

   ```

   uux "!diff a!/usr/file1 b!/a4/file2 >!~/file.diff"
   ```
2. The following command fails because *uux* places all files copied to a system in the same working directory. Although the
   files **xyz** are from two different systems, their filenames are the same and conflict.

   ```

   uux "!diff a!/usr1/xyz b!/usr2/xyz >!~/xyz.diff"
   ```
3. The following command succeeds (assuming [*diff*](../utilities/diff.html) is permitted on system **a**)
   because the file local to system **a** is not copied to the working directory, and hence does not conflict with the file from
   system **c**.

   ```

   uux "a!diff a!/usr/xyz c!/usr/xyz >!~/xyz.diff"
   ```

#### RATIONALE

None.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*2. Shell Command Language*](../utilities/V3_chap02.html#tag_19), [*uucp*](../utilities/uucp.html#), [*uuencode*](../utilities/uuencode.html#), [*uustat*](../utilities/uustat.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*11. General Terminal Interface*](../basedefs/V1_chap11.html#tag_11), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 6

The obsolescent SYNOPSIS is removed.

The normative text is reworded to avoid use of the term "must" for application requirements.

The UN margin code and associated shading are removed from the **-j** option in response to The Open Group Base Resolution
bwg2001-003.

#### Issue 7

SD5-XCU-ERN-46 is applied, moving this utility to the UUCP Utilities Option Group.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1516 is applied, adding XSI shading to text relating to *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
