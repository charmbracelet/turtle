# Special (GNU Coreutils 9.12)
#### 19.2.7 Special settings

‘`min n`’
:   Set the minimum number of characters that will satisfy a read until
    the time value has expired, when `-icanon` is set.

‘`time n`’
:   Set the number of tenths of a second before reads time out if the minimum
    number of characters have not been read, when `-icanon` is set.

‘`ispeed n`’
:   Set the input speed to n.

‘`ospeed n`’
:   Set the output speed to n.

‘`rows n`’
:   Tell the tty kernel driver that the terminal has n rows.

‘`cols n`’
:   Tell the kernel that the terminal has n columns.

‘`columns n`’
:   Same as ‘`cols`’. Non-POSIX.

‘`drain`’
:   Apply settings after first waiting for pending output to be transmitted.
    This is enabled by default for GNU `stty`.
    This is treated as an option rather than a line setting,
    and will follow the option processing rules described in the summary above.
    It is useful to disable this option
    in cases where the system may be in a state where serial transmission
    is not possible.
    For example, if the system has received the ‘`DC3`’ character
    with `ixon` (software flow control) enabled, then `stty` would
    block without `-drain` being specified.
    May be negated. Non-POSIX.

‘`size`’
:   Print the number of rows and columns that the kernel thinks the
    terminal has. (Systems that don’t support rows and columns in the kernel
    typically use the environment variables `LINES` and `COLUMNS`
    instead; however, GNU `stty` does not know anything about them.)

‘`line n`’
:   Use line discipline n. Non-POSIX.

‘`speed`’
:   Print the terminal speed.

‘`n`’
:   Set the input and output speeds to n. n can be one of: 0
    50 75 110 134 134.5 150 200 300 600 1200 1800 2400 4800 9600 19200
    38400 `exta` `extb`. `exta` is the same as 19200;
    `extb` is the same as 38400. Many systems, support arbitrary
    or higher speeds.
    0 hangs up the line if `-clocal` is set.
