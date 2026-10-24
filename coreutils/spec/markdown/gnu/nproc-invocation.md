# nproc invocation (GNU Coreutils 9.12)
### 21.3 `nproc`: Print the number of available processors

Print the number of processing units available to the current process,
which may be less than the number of online processors.
If this information is not accessible, then print the number of
processors installed. If the `OMP_NUM_THREADS` or `OMP_THREAD_LIMIT`
environment variables are set, then they will determine the minimum
and maximum returned value respectively. Linux cgroup version 2
CPU quotas may also limit the maximum returned value.
The result is guaranteed to be greater than zero. Synopsis:

```
nproc [option]
```

The program accepts the following options. Also see [Common options](Common-options.md).

‘`--all`’
:   Print the number of installed processors on the system, which may
    be greater than the number online or available to the current process.
    The `OMP_NUM_THREADS` or `OMP_THREAD_LIMIT` environment variables,
    or cgroup CPU quotas, are not honored in this case.

‘`--ignore=number`’
:   If possible, exclude this number of processing units.

An exit status of zero indicates success,
and a nonzero value indicates failure.
