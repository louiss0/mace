#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
output="$root/dist/musl-diagnostic"
rm -rf "$output"
mkdir -p "$output"
cd "$root"

export CGO_ENABLED=1
export CC=gcc
export GOTRACEBACK=crash

go version
ldd --version 2>&1 | head -n 1

run_under_gdb_on_failure() {
	local name="$1"
	shift

	echo "::group::$name"
	set +e
	"$@"
	local status=$?
	set -e
	if [[ $status -ne 0 ]]; then
		echo "$name failed with exit $status; collecting all thread backtraces" >&2
		gdb --quiet --batch --return-child-result \
			-ex 'set pagination off' \
			-ex run \
			-ex 'thread apply all backtrace full' \
			--args "$@" || true
		echo "::endgroup::"
		return "$status"
	fi
	echo "::endgroup::"
}

echo "::group::build trivial c-shared library"
go build -buildmode=c-shared \
	-o "$output/libtrivial.so" \
	./cmd/processor-abi/testdata/musl-trivial
gcc cmd/processor-abi/tests/musl_trivial_smoke.c \
	"$output/libtrivial.so" \
	-o "$output/trivial_smoke"
readelf -d "$output/libtrivial.so" | grep NEEDED || true
echo "::endgroup::"
run_under_gdb_on_failure "trivial Go c-shared call" "$output/trivial_smoke"

echo "::group::build processor c-shared library"
go build -buildmode=c-shared \
	-o "$output/libmace_processor.so" \
	./cmd/processor-abi
readelf -d "$output/libmace_processor.so" | grep NEEDED || true

gcc cmd/processor-abi/tests/abi_major_smoke.c \
	-Icmd/processor-abi \
	"$output/libmace_processor.so" \
	-o "$output/abi_major_smoke"
gcc cmd/processor-abi/tests/abi_smoke.c \
	-Icmd/processor-abi \
	"$output/libmace_processor.so" \
	-o "$output/abi_smoke"
echo "::endgroup::"

run_under_gdb_on_failure "processor ABI-major call" "$output/abi_major_smoke"
run_under_gdb_on_failure "complete processor C ABI" "$output/abi_smoke"
