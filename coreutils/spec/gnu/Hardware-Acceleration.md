# Hardware Acceleration (GNU Coreutils 9.12)
### 2.13 Hardware Acceleration Configuration

The `cksum` and `wc` utilities can use non-generic hardware
acceleration when available, which can significantly speed up processing. This
acceleration can be implemented internally within the coreutils themselves,
delegated to libraries like OpenSSL, or handled by the Linux kernel.

* [Build Configuration](#Build-Configuration)
* [Runtime Configuration](#Runtime-Configuration)

#### 2.13.1 Build Configuration

By default, the build enables hardware acceleration detection for all supported
features. This applies to both the accelerated code used internally within
coreutils and the accelerated code used in external libraries.

The detection performed internally within coreutils can be overridden at build
time using various configure variables, which currently include:

‘`gl_cv_crc_pclmul`’

‘`utils_cv_avx2_intrinsic_exists`’

‘`utils_cv_avx2_pclmul_intrinsic_exists`’

‘`utils_cv_avx512_pclmul_intrinsic_exists`’

‘`utils_cv_neon_intrinsic_exists`’

‘`utils_cv_pclmul_intrinsic_exists`’

‘`utils_cv_vmull_intrinsic_exists`’

External library detection is automatic for OpenSSL, so you must use the
‘`--with-openssl=no`’ configure option to disable this if desired.

#### 2.13.2 Runtime Configuration

The usual method for runtime configuration of hardware detection is through
environment variables, since you typically want to configure hardware
interactions quite broadly. Note that these environment variables are not
specific to coreutils, but coreutils will honor them when set.

For example, the following environment settings would disable hardware
acceleration both within coreutils and in libcrypto (if used) on x86_64,
ARM, and AArch64 platforms:

```
export OPENSSL_ia32cap='0x0'
export OPENSSL_armcap='0x0'
export GLIBC_TUNABLES='glibc.cpu.hwcaps=-ASIMD,-AVX512F,-AVX2,-AVX,-PMULL'
```

The `--debug` option is available on all utilities supporting
hardware acceleration and will report the current acceleration mode in use.
For example:

```
$ wc -l --debug /dev/null >/dev/null
wc: using avx2 hardware support

$ cksum --debug /dev/null >/dev/null
cksum: avx512 support not detected
cksum: avx2 support not detected
cksum: using pclmul hardware support
```
