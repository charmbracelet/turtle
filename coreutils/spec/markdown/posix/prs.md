# prs
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

prs — print an SCCS file (**DEVELOPMENT**)

#### SYNOPSIS

`[XSI] ![[Option Start]](../images/opt-start.gif) prs [-a] [-d dataspec]
[-r[SID]] file...

prs [-e|-l] -c cutoff [-d dataspec]
file...

prs [-e|-l] -r[SID] [-d dataspec]
file... ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

The *prs* utility shall write to standard output parts or all of an SCCS file in a user-supplied format.

#### OPTIONS

The *prs* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02), except that the **-r** option has an optional option-argument. This optional option-argument cannot be
presented as a separate argument. The following options shall be supported:

**-d***dataspec*
:   Specify the output data specification. The *dataspec* shall be a string consisting of SCCS file *data*
    *keywords* (see [Data Keywords](#tag_20_97_10_01)) interspersed with optional user-supplied text.

**-r[***SID***]**
:   Specify the SCCS identification string (SID) of a delta for which information is desired. If no *SID* option-argument is
    specified, the SID of the most recently created delta shall be assumed.

**-e**
:   Request information for all deltas created earlier than and including the delta designated via the **-r** option or the
    date-time given by the **-c** option.

**-l**
:   Request information for all deltas created later than and including the delta designated via the **-r** option or the
    date-time given by the **-c** option.

**-c***cutoff*
:   Indicate the *cutoff* date-time, in the form:
    ```

    YY[MM[DD[HH[MM[SS]]]]]
    ```

    For the *YY* component, values in the range [69,99] shall refer to years 1969 to 1999 inclusive, and values in the range
    [00,68] shall refer to years 2000 to 2068 inclusive.

    **Note:**
    :   It is expected that in a future version of this standard the default century inferred from a 2-digit year will change. (This
        would apply to all commands accepting a 2-digit year as input.)

    No changes (deltas) to the SCCS file that were created after the specified *cutoff* date-time shall be included in the
    output. Units omitted from the date-time default to their maximum possible values; for example, **-c 7502** is equivalent
    to **-c 750228235959**.

**-a**
:   Request writing of information for both removed—that is, [*delta*](../utilities/delta.html)
    *type*=*R* (see [*rmdel*](../utilities/rmdel.html#))—and existing—that is, [*delta*](../utilities/delta.html) *type*=*D*,—deltas. If the **-a** option is not specified, information for
    existing deltas only shall be provided.

#### OPERANDS

The following operand shall be supported:

*file*
:   A pathname of an existing SCCS file or a directory. If *file* is a directory, the *prs* utility shall behave as
    though each file in the directory were specified as a named file, except that non-SCCS files (last component of the pathname does
    not begin with **s.**) and unreadable files shall be silently ignored.

    If exactly one *file* operand appears, and it is '-', the standard input shall be read; each line of the standard
    input shall be taken to be the name of an SCCS file to be processed. Non-SCCS files and unreadable files shall be silently
    ignored.

#### STDIN

The standard input shall be a text file used only when the *file* operand is specified as '-'. Each line of the
text file shall be interpreted as an SCCS pathname.

#### INPUT FILES

Any SCCS files displayed are files of an unspecified format.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *prs*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as
    opposed to multi-byte characters in arguments and input files).

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written to standard error.

*NLSPATH*
:   Determine the location of messages objects and message catalogs.

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

The standard output shall be a text file whose format is dependent on the data keywords specified with the **-d** option.

##### Data Keywords

Data keywords specify which parts of an SCCS file shall be retrieved and output. All parts of an SCCS file have an associated
data keyword. A data keyword may appear in a *dataspec* multiple times.

The information written by *prs* shall consist of:

1. The user-supplied text
2. Appropriate values (extracted from the SCCS file) substituted for the recognized data keywords in the order of appearance in the
   *dataspec*

The format of a data keyword value shall either be simple ('S'), in which keyword substitution is direct, or multi-line
('M').

User-supplied text shall be any text other than recognized data keywords. A <tab> shall be specified by '\t' and
<newline> by '\n'. When the **-r** option is not specified, the default *dataspec* shall be:

```

:PN::\n\n
```

and the following *dataspec* shall be used for each selected delta:

```

:Dt:\t:DL:\nMRs:\n:MR:COMMENTS:\n:C:
```

| **SCCS File Data Keywords** | | | | |
| --- | --- | --- | --- | --- |
| **Keyword** | **Data Item** | **File Section** | **Value** | **Format** |
| **:Dt:** | Delta information | Delta Table | **See below\*** | S |
| **:DL:** | Delta line statistics | " | **:Li:/:Ld:/:Lu:** | S |
| **:Li:** | Lines inserted by Delta | " | ***nnnnn*\*\*\*** | S |
| **:Ld:** | Lines deleted by Delta | " | ***nnnnn*\*\*\*** | S |
| **:Lu:** | Lines unchanged by Delta | " | ***nnnnn*\*\*\*** | S |
| **:DT:** | Delta type | " | **D or R** | S |
| **:I:** | SCCS ID string (SID) | " | **See below\*\*** | S |
| **:R:** | Release number | " | ***nnnn*** | S |
| **:L:** | Level number | " | ***nnnn*** | S |
| **:B:** | Branch number | " | ***nnnn*** | S |
| **:S:** | Sequence number | " | ***nnnn*** | S |
| **:D:** | Date delta created | " | **:Dy:/:Dm:/:Dd:** | S |
| **:Dy:** | Year delta created | " | ***nn*** | S |
| **:Dm:** | Month delta created | " | ***nn*** | S |
| **:Dd:** | Day delta created | " | ***nn*** | S |
| **:T:** | Time delta created | " | **:Th:::Tm:::Ts:** | S |
| **:Th:** | Hour delta created | " | ***nn*** | S |
| **:Tm:** | Minutes delta created | " | ***nn*** | S |
| **:Ts:** | Seconds delta created | " | ***nn*** | S |
| **:P:** | Programmer who created Delta | " | ***logname*** | S |
| **:DS:** | Delta sequence number | " | ***nnnn*** | S |
| **:DP:** | Predecessor Delta sequence number | " | ***nnnn*** | S |
| **:DI:** | Sequence number of deltas included, excluded, or ignored | " | **:Dn:/:Dx:/:Dg:** | S |
| **:Dn:** | Deltas included (sequence #) | " | **:DS: :DS: ...** | S |
| **:Dx:** | Deltas excluded (sequence #) | " | **:DS: :DS: ...** | S |
| **:Dg:** | Deltas ignored (sequence #) | " | **:DS: :DS: ...** | S |
| **:MR:** | MR numbers for delta | " | ***text*** | M |
| **:C:** | Comments for delta | " | ***text*** | M |
| **:UN:** | User names | User Names | ***text*** | M |
| **:FL:** | Flag list | Flags | ***text*** | M |
| **:Y:** | Module type flag | " | ***text*** | S |
| **:MF:** | MR validation flag | " | **yes or no** | S |
| **:MP:** | MR validation program name | " | ***text*** | S |
| **:KF:** | Keyword error, warning flag | " | **yes or no** | S |
| **:BF:** | Branch flag | " | **yes or no** | S |
| **:J:** | Joint edit flag | " | **yes or no** | S |
| **:LK:** | Locked releases | " | **:R: ...** | S |
| **:Q:** | User-defined keyword | " | ***text*** | S |
| **:M:** | Module name | " | ***text*** | S |
| **:FB:** | Floor boundary | " | **:R:** | S |
| **:CB:** | Ceiling boundary | " | **:R:** | S |
| **:Ds:** | Default SID | " | **:I:** | S |
| **:ND:** | Null delta flag | " | **yes or no** | S |
| **:FD:** | File descriptive text | Comments | ***text*** | M |
| **:BD:** | Body | Body | ***text*** | M |
| **:GB:** | Gotten body | " | ***text*** | M |
| **:W:** | A form of *what* string | N/A | **:Z::M:\t:I:** | S |
| **:A:** | A form of *what* string | N/A | **:Z::Y: :M: :I::Z:** | S |
| **:Z:** | *what* string delimiter | N/A | **@(#)** | S |
| **:F:** | SCCS filename | N/A | ***text*** | S |
| **:PN:** | SCCS file pathname | N/A | ***text*** | S |

\*
:   **:Dt:**=**:DT: :I: :D: :T: :P: :DS: :DP:**

\*\*
:   **:R:.:L:.:B:.:S:** if the delta is a branch delta (**:BF:**==**yes**)
    **:R:.:L:** if the delta is not a branch delta (**:BF:**==**no**)

\*\*\*
:   The line statistics are capped at 99999. For example, if 100000 lines were unchanged in a certain revision, **:Lu:** shall
    produce the value 99999.

#### STDERR

The standard error shall be used only for diagnostic messages.

#### OUTPUT FILES

None.

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

None.

#### EXAMPLES

1. The following example:
   ```

   prs -d "User Names for :F: are:\n:UN:" s.file
   ```

   might write to standard output:

   ```

   User Names for s.file are:
   xyz
   131
   abc
   ```
2. The following example:
   ```

   prs -d "Delta for pgm :M:: :I: - :D: By :P:" -r s.file
   ```

   might write to standard output:

   ```

   Delta for pgm main.c: 3.7 - 77/12/01 By cas
   ```
3. As a special case:
   ```

   prs s.file
   ```

   might write to standard output:

   ```

   s.file:
   <blank line>
   D 1.1 77/12/01 00:00:00 cas 1 000000/00000/00000
   MRs:
   bl78-12345
   bl79-54321
   COMMENTS:
   this is the comment line for s.file initial delta
   <blank line>
   ```

   for each delta table entry of the **D** type. The only option allowed to be used with this special case is the
   **-a** option.

#### RATIONALE

None.

#### FUTURE DIRECTIONS

If this utility is directed to display a pathname that contains any bytes that have the encoded value of a <newline>
character when <newline> is a terminator or separator in the output format being used, implementations are encouraged to
treat this as an error. A future version of this standard may require implementations to treat this as an error.

#### SEE ALSO

[*admin*](../utilities/admin.html#), [*delta*](../utilities/delta.html#), [*get*](../utilities/get.html#), [*what*](../utilities/what.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 5

The phrase "in which keyword substitution is followed by a <newline>" is deleted from the end of the second paragraph of
[Data Keywords](#tag_20_97_10_01).

The interpretation of the *YY* component of the **-c** *cutoff* argument is noted.

#### Issue 6

The normative text is reworded to emphasize the term "shall" for implementation requirements.

The Open Group Base Resolution bwg2001-007 is applied, updating the table in STDOUT with a note that line
statistics are capped at 99999 for the **:Li:**, **:Ld:**, **:Lu:**, and **:DL:** keywords.

The Open Group Interpretation PIN4C.00009 is applied.

#### Issue 7

SD5-XCU-ERN-97 is applied, updating the SYNOPSIS.

#### Issue 8

Austin Group Defect 251 is applied, encouraging implementations to report an error if a utility is directed to display a
pathname that contains any bytes that have the encoded value of a <newline> character when <newline> is a terminator or
separator in the output format being used.

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1452 is applied, deleting **:KV:** from the list of keywords.

Austin Group Defect 1570 is applied, removing extra spacing in "==".

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
