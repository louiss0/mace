/* Windows has no import library for a Go c-shared DLL, and generating one needs
   tools that are not guaranteed on a runner. Loading the library by name proves
   the same thing the linked POSIX test does: that every symbol is exported with
   C linkage and behaves as the header declares. */

#include "processor.h"
#include <assert.h>
#include <stdio.h>
#include <windows.h>

static HMODULE library;

#define BIND(name) \
    static mace_handle(APIENTRY *name); \
    name = (mace_handle(APIENTRY *) GetProcAddress(library, #name); \
    if (name == NULL) { \
        fprintf(stderr, "missing export: %s\n", #name); \
        return 2; \
    }

static uint32_t(APIENTRY *p_abi_major)(void);
static mace_handle(APIENTRY *p_process_source)(const char *, const char *, const char *);
static mace_handle(APIENTRY *p_process_file)(const char *, const char *, const char *);
static mace_handle(APIENTRY *p_request_new)(uint32_t);
static mace_handle(APIENTRY *p_process_source_with_request)(mace_handle, const char *, const char *, const char *);
static void(APIENTRY *p_request_free)(mace_handle);
static mace_handle(APIENTRY *p_result_root)(mace_handle);
static char *(APIENTRY *p_result_error)(mace_handle);
static void(APIENTRY *p_result_free)(mace_handle);
static uint32_t(APIENTRY *p_value_kind)(mace_handle);
static int64_t(APIENTRY *p_value_int)(mace_handle);
static uint64_t(APIENTRY *p_value_record_length)(mace_handle);
static char *(APIENTRY *p_value_record_key)(mace_handle, uint64_t);
static uint64_t(APIENTRY *p_value_record_key_length)(mace_handle, uint64_t);
static mace_handle(APIENTRY *p_value_record_value)(mace_handle, uint64_t);
static uint64_t(APIENTRY *p_string_length)(const char *);
static void(APIENTRY *p_string_free)(char *);

static int bind_exports(void) {
    struct {
        const char *name;
        FARPROC *slot;
    } exports[] = {
        {"mace_abi_major", (FARPROC *) &p_abi_major},
        {"mace_process_source", (FARPROC *) &p_process_source},
        {"mace_process_file", (FARPROC *) &p_process_file},
        {"mace_request_new", (FARPROC *) &p_request_new},
        {"mace_process_source_with_request", (FARPROC *) &p_process_source_with_request},
        {"mace_request_free", (FARPROC *) &p_request_free},
        {"mace_result_root", (FARPROC *) &p_result_root},
        {"mace_result_error", (FARPROC *) &p_result_error},
        {"mace_result_free", (FARPROC *) &p_result_free},
        {"mace_value_kind", (FARPROC *) &p_value_kind},
        {"mace_value_int", (FARPROC *) &p_value_int},
        {"mace_value_record_length", (FARPROC *) &p_value_record_length},
        {"mace_value_record_key", (FARPROC *) &p_value_record_key},
        {"mace_value_record_key_length", (FARPROC *) &p_value_record_key_length},
        {"mace_value_record_value", (FARPROC *) &p_value_record_value},
        {"mace_string_length", (FARPROC *) &p_string_length},
        {"mace_string_free", (FARPROC *) &p_string_free},
    };

    for (size_t index = 0; index < sizeof(exports) / sizeof(exports[0]); index += 1) {
        FARPROC address = GetProcAddress(library, exports[index].name);
        if (address == NULL) {
            fprintf(stderr, "missing export: %s\n", exports[index].name);
            return 2;
        }
        *exports[index].slot = address;
    }

    return 0;
}

int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "usage: abi_smoke_windows <path-to-dll>\n");
        return 2;
    }

    library = LoadLibraryA(argv[1]);
    if (library == NULL) {
        fprintf(stderr, "unable to load %s (error %lu)\n", argv[1], GetLastError());
        return 2;
    }
    if (bind_exports() != 0) {
        return 2;
    }

    assert(p_abi_major() == 1);

    mace_handle result = p_process_source("[output = 'data']\n{ count: 42, enabled: true, }", ".", 0);
    assert(result != 0);
    char *failure = p_result_error(result);
    assert(failure == NULL);

    mace_handle record = p_result_root(result);
    assert(p_value_kind(record) == MACE_RECORD);
    assert(p_value_record_length(record) == 2);
    for (uint64_t index = 0; index < 2; index += 1) {
        char *key = p_value_record_key(record, index);
        assert(key != 0);
        assert(p_value_record_key_length(record, index) == p_string_length(key));
        p_string_free(key);
    }
    assert(p_value_kind(p_value_record_value(record, 0)) == MACE_INT);
    assert(p_value_int(p_value_record_value(record, 0)) == 42);
    p_result_free(result);

    mace_handle request = p_request_new(5000);
    assert(request != 0);
    mace_handle requested = p_process_source_with_request(request, "[output = 'data']\n{ ok: true, }", ".", 0);
    assert(requested != 0);
    assert(p_result_error(requested) == NULL);
    p_result_free(requested);
    p_request_free(request);

    mace_handle invalid = p_process_source("{ nope: }", ".", 0);
    assert(invalid != 0);
    char *message = p_result_error(invalid);
    assert(message != 0);
    assert(p_string_length(message) > 0);
    p_string_free(message);
    p_result_free(invalid);

    mace_handle outside = p_process_file("../../outside.mace", ".", 0);
    assert(outside != 0);
    char *rejected = p_result_error(outside);
    assert(rejected != 0);
    p_string_free(rejected);
    p_result_free(outside);

    FreeLibrary(library);
    printf("windows c abi ok\n");
    return 0;
}
