## abort.debug

`abort.debug` was produced by compiling the following code with
`x86_64-pc-linux-gnu-gcc -g -o abort.debug abort.c`:

```c
#include <stdlib.h>

int main(int argc, char** argv) {
  abort();
}
```

Compiler version info:

```sh
$ i686-pc-linux-gnu-gcc --version | head -1
i686-pc-linux-gnu-gcc.real (4.9.2_cos_gg_4.9.2-r175-0c5a656a1322e137fa4a251f2ccc6c4022918c0a_4.9.2-r175) 4.9.x 20150123 (prerelease)
```

`abort.debug` was then copied to `abort`, and `strip abort` was executed to
remove debugging symbols. The program was then copied to a lumpy device and
executed at `/usr/local/bin/abort` to produce
`/var/spool/crash/abort.20180103.145440.12345.20827.dmp`.

## chrome\_crash\_report.dmp

`chrome_crash_report.dmp` is the first 4096 bytes of a browser crash report
written by Chrome. The minidump length "519824" starting at 0x51C was changed to
"002781" to reflect the minidump data's truncation to (0x1000 - 0x523 = 2781)
bytes.

## chrome.20210406.155226.62533.9042.dmp

`chrome.20210406.155226.62533.9042.dmp` was produced by visiting chrome:crash
on betty VM.

## chrome.20210706.000145.15087.8090.dmp

`chrome.20210706.000145.15087.8090.dmp` was produced by visiting chrome:crash
on betty VM. It contains Crashpad annotations.

## lacros\_debug.zip

lacros\_debug.zip simulates a real file found on gs://. It was created from
a simple C++ program.

```c
// chrome.cc
#include <iostream>

int main(int argc, char* argv[]) {
  return 0;
}
```

The program was compiled as follows:

```sh
g++ -g chrome.cc -o chrome.debug
dump_syms -i chrome.debug # Find the module ID, which we verify in the test.
zip lacros_debug.zip chrome.debug
```

