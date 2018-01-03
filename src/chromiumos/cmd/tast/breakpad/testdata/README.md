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
`/var/spool/crash/abort.20180103.145440.20827.dmp`.
