# Known limitation: Go `c-shared` does not support this musl ABI

**Status:** diagnosed, not fixable in the processor ABI with the stock Go
1.25.5 toolchain. musl remains excluded from processor releases.

**Severity:** high. A startup-linked library segfaults during Go runtime
initialization, before an exported function runs. A dynamically loaded library,
which is how the official language bindings use it, is rejected by musl's
loader.

**Discovered:** 2026-09-27 by the processor release C smoke test.

**Diagnosed:** 2026-09-28 by native Alpine run
[`36470927933`](https://github.com/louiss0/mace/actions/runs/36470927933).

## Root causes

There are two independent upstream Go runtime/toolchain limitations.

### 1. Go assumes shared-library constructors receive `argc` and `argv`

Go's Linux `c-shared` startup path expects its ELF initialization function to
receive the glibc-specific `(argc, argv, envp)` arguments. ELF does not require
those arguments, and musl does not provide them. The Go runtime consequently
receives a null `argv` and a garbage `argc`, then dereferences them in
`runtime.sysargs`.

A trivial library exporting only `trivial_answer() == 42` fails before that
function executes:

```text
Thread 2 "trivial_smoke" received signal SIGSEGV, Segmentation fault.
runtime.argv_index (argv=0x0, i=-135161095)
runtime.sysargs (argc=-135161096, argv=0x0)
runtime.args(...)
runtime.rt0_go()
```

This is the failure tracked by
[`golang/go#13492`](https://github.com/golang/go/issues/13492). The issue has
been open since 2015. The duplicate
[`golang/go#19016`](https://github.com/golang/go/issues/19016) contains the same
`runtime.sysargs` backtrace from a trivial Alpine library.

### 2. Go emits initial-exec TLS, which musl rejects for `dlopen`

The official Python, Node, and Dart bindings load the processor at runtime.
A minimal C program using `dlopen(..., RTLD_NOW)` cannot load even the trivial
library:

```text
dlopen failed: Error relocating libtrivial.so: (null):
initial-exec TLS resolves to dynamic definition in libtrivial.so
```

Initial-exec TLS requires the shared object to be present when the process
starts; it is not valid for a library introduced later by `dlopen`. This
separate toolchain limitation is tracked by
[`golang/go#54805`](https://github.com/golang/go/issues/54805).

An open upstream proposal,
[`golang/go#75048`](https://github.com/golang/go/pull/75048), attempts to solve
both limitations, but it is not merged and has reports of continuing
segfaults. Processor releases must use a released, unmodified Go toolchain, so
that proposal is not a release dependency.

## Evidence that this is not a Mace ABI defect

The native Alpine diagnostic in
[`tests/musl_smoke.sh`](tests/musl_smoke.sh) separates runtime startup from
processor behavior:

1. It builds a trivial Go `c-shared` library with one integer-returning export.
2. A startup-linked C caller segfaults in `runtime.argv_index`.
3. An unlinked C caller receives the initial-exec TLS error from `dlopen`.
4. A caller linked to the processor library and invoking only
   `mace_abi_major()` segfaults in the same runtime frame.
5. The complete `abi_smoke.c` test fails in that same frame.

The library builds, links, and is a valid musl ELF object. No processor code or
C ABI argument handling executes before the startup-linked fault. Reproducing
both failures under `golang:1.25.5-alpine` also rules out contamination from the
previous Ubuntu/glibc cross-build environment.

## Resolution

There is no safe library-level workaround that preserves the direct C ABI:

- Supplying safe constructor arguments would address only the first failure;
  dynamic loading would still fail because of the TLS model.
- Preloading the library would change the binding installation and process
  startup contract and cannot be imposed by a package.
- Shipping a locally patched Go runtime would create an unsupported toolchain
  fork and still requires a proven dynamic-TLS implementation.
- Reintroducing a CLI sidecar would abandon the direct native-library design.

Therefore `linux-amd64-musl` and `linux-arm64-musl` are explicitly unsupported,
rather than shipping libraries known to crash or fail to load. Alpine users
must use a glibc-based environment for the native bindings. This is a platform
constraint in Go's `c-shared` output, not a processor implementation bug.

A future released Go version may make musl viable. Before adding either target
back to `processor-targets.json`, all of these must pass on native musl for both
architectures:

1. the trivial startup-linked C test;
2. the trivial `dlopen` C test;
3. the processor ABI-major C test;
4. the complete processor `abi_smoke.c` test; and
5. each installed binding's native package tests.

## Reproduction

Run the diagnostic workflow manually, or use a native Alpine environment:

```sh
apk add --no-cache bash binutils build-base gdb git
bash cmd/processor-abi/tests/musl_smoke.sh
```

For the original cross-build reproduction on a Debian-compatible Linux host:

```sh
sudo apt-get update && sudo apt-get install -y musl-tools
CGO_ENABLED=1 CC=musl-gcc go build -buildmode=c-shared \
  -o dist/libmace_processor.so ./cmd/processor-abi
sudo ln -sf /usr/lib/x86_64-linux-musl/libc.so /lib/ld-musl-x86_64.so.1
musl-gcc cmd/processor-abi/tests/abi_smoke.c -Icmd/processor-abi \
  dist/libmace_processor.so -o dist/abi_smoke
./dist/abi_smoke # Segmentation fault, exit code 139
```

## Do not

- Do not publish a musl target merely because it builds.
- Do not treat an `argc`/`argv` patch as sufficient; bindings require `dlopen`.
- Do not hand-place a musl library in a binding package.
- Do not re-enable musl until every gate in the resolution section passes.
