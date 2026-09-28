# Known failure: the musl C ABI segfaults

**Status:** open, unresolved. musl is excluded from the release. Do not add it
back to `processor-targets.json` until this is fixed and the C smoke test passes
on a musl runner.

**Severity:** high. A shared library that segfaults a plain C caller is a
memory-safety fault. It must be understood, not worked around.

**Discovered:** 2026-09-27, by the C smoke test in the processor release
workflow. It was not found by inspection or by the Go test suite.

## What happens

A c-shared library built against musl compiles, links, and loads. The first C
program to call into it dies with SIGSEGV.

From the release workflow, `linux-amd64-musl` at commit `74756bf`:

```
musl-gcc cmd/processor-abi/tests/abi_smoke.c -Icmd/processor-abi "$LIBRARY" -o "dist/$TARGET/abi_smoke"
/…/abi_smoke.sh: line 11: 4505 Segmentation fault (core dumped) "dist/$TARGET/abi_smoke"
##[error]Process completed with exit code 139.
```

Exit 139 is 128 + SIGSEGV(11). The same failure occurs for `linux-arm64-musl`.

## What is ruled out

- **The library does not build wrong.** `go build -buildmode=c-shared` with
  `CC=musl-gcc` succeeds. The artifact exists and is well formed.
- **The smoke test is not wrong.** The identical C source links and passes
  against the glibc library on all four glibc/macOS/Windows targets. It also
  passed against a locally built Windows library, and it demonstrably catches
  deliberate regressions (mutating `mace_value_record_length` and
  `mace_value_record_key` made it fail, as did a bad `processor.def`).
- **It is not the loader.** An earlier attempt failed with exit 126/127 because
  the musl loader was missing. That was a *different*, understood problem: a
  musl-linked test program needs `/lib/ld-musl-x86_64.so.1`, and the glibc CI
  runner does not provide it. Once a symlink to
  `/usr/lib/x86_64-linux-musl/libc.so` was created, the program started and then
  segfaulted. The current failure is past that point.
- **It is not a Zig cross-compilation problem.** An earlier build used
  `zig cc -target x86_64-linux-musl` and failed to link
  (`ld.lld: undefined symbol: main`). That path was abandoned in favour of
  `musl-gcc`, which builds cleanly.
- **It is not the release pipeline.** Staging, checksums, and the manifest are
  downstream of the build and were never reached.

## What is NOT known

The actual cause. Nothing has been diagnosed beyond the signal above, and
**this must not be reported as understood.** The open questions:

1. Is the fault in this ABI's C surface, or in the Go/musl runtime underneath
   it? A cgo library built with musl-gcc links a musl libc into a Go runtime
   that has its own assumptions about the platform. A mismatch there is a
   plausible cause and would be a Go/musl interaction, not an ABI bug.
2. Is it specific to the C caller, or does it also affect ctypes, Node (Koffi),
   and Dart (FFI)? Each of those binds differently. Only the C path was
   observed, because only the C path was run on musl.
3. Does it reproduce with a trivial cgo `-buildmode=c-shared` library that
   exports one no-op function? That is the single most useful next experiment:
   it separates "this ABI is broken on musl" from "c-shared plus musl is broken
   here".
4. Does it reproduce on a real Alpine container, or only on a glibc Debian
   runner with a musl sysroot? The CI runner is the latter, which is an
   unusual configuration that may itself be the trigger.

## How to reproduce

Requires a Linux x64 or arm64 host (or CI runner) with `musl-tools` installed.

```sh
# 1. Build the library against musl.
sudo apt-get update && sudo apt-get install -y musl-tools
CGO_ENABLED=1 CC=musl-gcc go build -buildmode=c-shared \
  -o dist/libmace_processor.so ./cmd/processor-abi

# 2. A musl-linked test program needs the musl loader, which a glibc host
#    does not provide under the expected name.
sudo ln -sf /usr/lib/x86_64-linux-musl/libc.so /lib/ld-musl-x86_64.so.1

# 3. Build and run the C smoke test against it.
musl-gcc cmd/processor-abi/tests/abi_smoke.c -Icmd/processor-abi \
  dist/libmace_processor.so -o dist/abi_smoke
./dist/abi_smoke        # segfaults, exit 139
```

Expect a core dump. `gdb ./dist/abi_smoke` on the core, or run the failing
binary under `gdb` directly, to get the faulting frame. That frame is the
missing piece of evidence.

## What was done instead

musl was removed from `processor-targets.json` and moved to the `unsupported`
map with this reason recorded. `cmd/processor-abi/targets_test.go` asserts that
every excluded platform has a documented reason and is not also published, so
this gap cannot be silently dropped. All three bindings refuse to load a
library on `linux-*-musl` and raise a clear "not published for this platform"
error instead of attempting a download. The five published platforms
(macOS and Windows x64/arm64, Linux x64/arm64 glibc) are unaffected and are
covered by the C smoke test and the Python ABI harness on every build.

## Do not

- Do not add a musl target back to the release, even "just to test the plumbing".
- Do not hand-place a musl library into a binding's `bin/` directory. The
  bindings reject unsupported platforms precisely so this cannot happen quietly.
- Do not mark this resolved without a C smoke test that passes on musl.
