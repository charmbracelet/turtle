# Signal specifications (GNU Coreutils 9.12)
### 2.3 Signal specifications

A signal may be a signal name like ‘`HUP`’, or a signal
number like ‘`1`’, or an exit status of a process terminated by the
signal. A signal name can be given in canonical form or prefixed by
‘`SIG`’. The case of the letters is ignored.

The signal ‘`0`’ pseudo signal is synonymous with the name ‘`EXIT`’
with the GNU `kill` command, and `bash` at least,
as `trap foo 0` and `trap foo EXIT` are equivalent.

The following signal names and numbers are supported
on all POSIX compliant systems:

‘`HUP`’
:   1. Hangup.

‘`INT`’
:   2. Terminal interrupt.

‘`QUIT`’
:   3. Terminal quit.

‘`ABRT`’
:   6. Process abort.

‘`KILL`’
:   9. Kill (cannot be caught or ignored).

‘`ALRM`’
:   14. Alarm Clock.

‘`TERM`’
:   15. Termination.

Other supported signal names have system-dependent corresponding
numbers. All systems conforming to POSIX 1003.1-2001 also
support the following signals:

‘`BUS`’
:   Access to an undefined portion of a memory object.

‘`CHLD`’
:   Child process terminated, stopped, or continued.

‘`CONT`’
:   Continue executing, if stopped.

‘`FPE`’
:   Erroneous arithmetic operation.

‘`ILL`’
:   Illegal Instruction.

‘`PIPE`’
:   Write on a pipe with no one to read it.

‘`SEGV`’
:   Invalid memory reference.

‘`STOP`’
:   Stop executing (cannot be caught or ignored).

‘`TSTP`’
:   Terminal stop.

‘`TTIN`’
:   Background process attempting read.

‘`TTOU`’
:   Background process attempting write.

‘`URG`’
:   High bandwidth data is available at a socket.

‘`USR1`’
:   User-defined signal 1.

‘`USR2`’
:   User-defined signal 2.

POSIX 1003.1-2001 systems that support the XSI extension
also support the following signals:

‘`POLL`’
:   Pollable event.

‘`PROF`’
:   Profiling timer expired.

‘`SYS`’
:   Bad system call.

‘`TRAP`’
:   Trace/breakpoint trap.

‘`VTALRM`’
:   Virtual timer expired.

‘`XCPU`’
:   CPU time limit exceeded.

‘`XFSZ`’
:   File size limit exceeded.

POSIX 1003.1-2001 systems that support the XRT extension
also support at least eight real-time signals called ‘`RTMIN`’,
‘`RTMIN+1`’, …, ‘`RTMAX-1`’, ‘`RTMAX`’.

All systems conforming to POSIX 1003.1-2024 also support the following
signals:

‘`WINCH`’
:   Terminal window size changed.
