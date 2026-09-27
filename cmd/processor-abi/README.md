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

Use `mace_request_new(timeout_ms)` and the `_with_request` entrypoints for
per-call deadlines or cancellation from another thread. The default deadline
is 30 seconds. The request must be freed after its result; cancelled calls
return a diagnostic instead of a value. Remote file imports, schema-file
reads, import resolution, and every value evaluation share the request's
deadline, so a long evaluation stops at the next expression checkpoint. A
synchronous local-file read is the remaining stage that cannot be interrupted
mid-call.

## Releasing processor artifacts

Processor artifacts ship on their own `processor/vX.Y.Z` cadence, separately
from the `vX.Y.Z` CLI release. [`processor-targets.json`](../../processor-targets.json)
is the canonical list of the eight supported variants and is verified by
`cmd/processor-abi/targets_test.go`. `.github/workflows/release-processor.yml`
builds each variant on its native runner, runs `abi_test.py` against the
freshly built library, and refuses to publish unless all eight variants
upload successfully. The published release contains
`processor-manifest.json` (version, target, artifact path, SHA-256) plus
`checksums.txt`, and every staged artifact carries a build-provenance
attestation. Bindings pin a version and verify the manifest hash before
staging a library.

**Migration status:** The eight-variant release gate now exists but has not run
end to end, and installed-package tests do not yet cover every variant in a
single run.
