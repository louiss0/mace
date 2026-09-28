# Processor C ABI (official bindings)

`./cmd/processor-abi` builds a Go `-buildmode=c-shared` library. The C interface
is defined in [`processor.h`](processor.h). It is an internal integration
contract for the official Node, Python, and Dart bindings, not a separately
supported third-party SDK. `mace_abi_major()` lets each binding reject an
incompatible library before calling any other function.

Build for the local OS with a working C compiler and cgo:

```sh
CGO_ENABLED=1 go build -buildmode=c-shared -o libmace_processor.so ./cmd/processor-abi
MACE_PROCESSOR_LIBRARY="$PWD/libmace_processor.so" python3 cmd/processor-abi/abi_test.py
```

On macOS use `libmace_processor.dylib`; on Windows use `mace_processor.dll`.

`mace_process_file` evaluates a `.mace` file inside the workspace root;
`mace_process_source` evaluates source in memory, resolving imports against
the workspace root. Each accepts an optional Mace record-literal input, as a
UTF-8 string. A null workspace uses the calling process's working directory.
On success `mace_result_root` is an output-record handle; on failure
`mace_result_error` is a non-null diagnostic message and the root is zero.
Diagnostic kind, code, and source positions are available separately.

A result owns the complete immutable tree of value handles. Call
`mace_result_free` once after traversal; no child handle may be used after
that. Concurrent evaluations and read-only traversal of live trees are safe,
but callers must not free a result while another thread reads it. Records are
indexed in sorted key order; arrays preserve source order. Scalar kinds are
specified in `processor.h`. Hex numbers expose their formatted representation
through `mace_value_string` and preserve their kind separately. String
pointers returned from getters are independent C allocations: read their
reported byte length where available and always release them with
`mace_string_free`. Do not free value handles individually.

The registry that backs the handles holds at most `liveHandleLimit` entries. A
caller that stops freeing results and requests does not grow the process until
it is killed: every later evaluation fails with a `mace_result_error` of
"too many live processor handles". Recover by releasing handles; the overflow
result is a single shared handle, so the failure is bounded.

Use `mace_request_new(timeout_ms)` and the `_with_request` entrypoints for
per-call deadlines or cancellation from another thread. The default deadline
is 30 seconds. The request must be freed after its result; cancelled calls
return a diagnostic instead of a value. Remote file imports, schema-file
reads, import resolution, and every value evaluation share the request's
deadline, so a long evaluation stops at the next expression checkpoint. A
synchronous local-file read is the remaining stage that cannot be interrupted
mid-call.

## Remote imports are not a sandbox

The workspace check bounds the entry file and every local import after
symlinks resolve. HTTP(S) imports are deliberately outside that check, so a
`.mace` file can name any URL, including a cloud metadata endpoint, and the
host will issue the request. Treat an untrusted `.mace` file as untrusted
network input, not merely as untrusted data.

## Releasing processor artifacts

Processor artifacts ship on their own `processor/vX.Y.Z` cadence, separately
from the `vX.Y.Z` CLI release. [`processor-targets.json`](../../processor-targets.json)
is the canonical list of supported variants and is verified by
`cmd/processor-abi/targets_test.go`. `.github/workflows/release-processor.yml`
builds each variant on its native runner, compiles and runs the C smoke test
plus `abi_test.py` against the freshly built library, and refuses to publish
unless every listed variant uploads successfully. The published release
contains `processor-manifest.json` (version, target, artifact path, SHA-256)
plus `checksums.txt`, and every staged artifact carries a build-provenance
attestation. After publication, the workflow verifies the release through the
bindings' staging script, updates the pinned manifest digest on a bot branch,
and opens a `mace-bindings` pull request. That pull request triggers the
bindings' full platform CI before the pin reaches its main branch.

## Platforms that are not built

Five variants are published. Three platforms are deliberately excluded, and
`processor-targets.json` records the reason for each under `unsupported`:

| Platform | Why it is not published |
| --- | --- |
| `linux-amd64-musl`, `linux-arm64-musl` | The library builds with `musl-gcc`, but a C program that calls it **segfaults**. The cause is not yet understood, so no musl artifact may ship. |
| `windows-arm64` | The `windows-11-arm` runner has no aarch64 mingw sysroot, so cgo cannot resolve `stdlib.h` for a cross build. A cross toolchain is required first. |

The musl segfault is the important one. It was found by the C smoke test in
CI, not by inspection, and it is unresolved: it is not yet known whether the
fault is in this ABI or in the musl build. **[`musl-segfault.md`](musl-segfault.md)
records the full failure, the reproduction, and what is ruled out.** Until that
is answered, do not add musl back to the target list, and do not publish a
hand-built musl library.

**Migration status:** The release gate exists and has published five of the
eight intended variants. `linux-amd64-musl`, `linux-arm64-musl`, and
`windows-arm64` are excluded and documented above. The musl segfault must be
resolved before musl can ship; see [`musl-segfault.md`](musl-segfault.md).
