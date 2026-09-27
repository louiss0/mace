#include "processor.h"
#include <assert.h>
#include <stdint.h>
#include <string.h>

/* Exercises the ownership and shape of the exported surface through a real C
   toolchain: value traversal, borrowed string lifetimes, the error path, the
   request lifecycle, and the workspace boundary. Every pointer this test
   receives from the library must be released with mace_string_free. */

static void assert_record_keys_are_sorted(mace_handle record, uint64_t expected) {
    assert(mace_value_kind(record) == MACE_RECORD);
    assert(mace_value_record_length(record) == expected);

    for (uint64_t index = 0; index < expected; index += 1) {
        char *key = mace_value_record_key(record, index);
        assert(key != 0);
        assert(mace_value_record_key_length(record, index) == mace_string_length(key));
        mace_string_free(key);

        mace_handle child = mace_value_record_value(record, index);
        assert(child != 0);
        assert(mace_value_kind(child) != MACE_UNKNOWN);
    }
}

/* Records are indexed in sorted key order, so { count, enabled, name, ratio }
   is read positionally rather than in source order. A null field is omitted
   from an output record entirely, so null is asserted through an array. */
static void assert_typed_values(mace_handle record) {
    mace_handle count = mace_value_record_value(record, 0);
    assert(mace_value_kind(count) == MACE_INT);
    assert(mace_value_int(count) == 42);

    mace_handle enabled = mace_value_record_value(record, 1);
    assert(mace_value_kind(enabled) == MACE_BOOLEAN);
    assert(mace_value_boolean(enabled) == 1);

    mace_handle name = mace_value_record_value(record, 2);
    assert(mace_value_kind(name) == MACE_STRING);
    char *text = mace_value_string(name);
    assert(text != 0);
    assert(mace_value_string_length(name) == mace_string_length(text));
    assert(memcmp(text, "Ada", 3) == 0);
    mace_string_free(text);

    mace_handle ratio = mace_value_record_value(record, 3);
    assert(mace_value_kind(ratio) == MACE_FLOAT);
    assert(mace_value_float(ratio) > 0.5 && mace_value_float(ratio) < 1.5);
}

/* A null output field is dropped before it reaches the ABI, so MACE_NULL is not
   reachable through a successful evaluation. Handle zero is not a value either. */
static void assert_null_fields_are_dropped(void) {
    mace_handle result = mace_process_source("[output = 'data']\n{ absent: null, present: 1, }", ".", 0);
    assert(result != 0);
    assert(mace_result_error(result) == 0);

    mace_handle record = mace_result_root(result);
    assert_record_keys_are_sorted(record, 1);
    assert(mace_value_kind(mace_value_record_value(record, 0)) == MACE_INT);
    assert(mace_value_kind(0) == MACE_UNKNOWN);
    mace_result_free(result);
}

static void assert_nested_arrays(mace_handle record) {
    mace_handle tags = mace_value_record_value(record, 0);
    assert(mace_value_kind(tags) == MACE_ARRAY);
    assert(mace_value_array_length(tags) == 2);

    mace_handle first = mace_value_array_item(tags, 0);
    assert(mace_value_kind(first) == MACE_STRING);
    char *text = mace_value_string(first);
    assert(memcmp(text, "a", 1) == 0);
    mace_string_free(text);

    mace_handle second = mace_value_array_item(tags, 1);
    assert(mace_value_kind(second) == MACE_INT);
    assert(mace_value_int(second) == 7);

    /* Reading past the end must return the null handle, never a wild pointer. */
    assert(mace_value_array_item(tags, 99) == 0);
    assert(mace_value_record_key(tags, 0) == 0);
    assert(mace_string_length(0) == 0);
}

static void assert_evaluation(char *source) {
    mace_handle result = mace_process_source(source, ".", 0);
    assert(result != 0);
    assert(mace_result_error(result) == 0);

    mace_handle record = mace_result_root(result);
    assert_record_keys_are_sorted(record, 4);
    assert_typed_values(record);
    mace_result_free(result);
}
static void assert_error_path_releases_its_message(void) {
    mace_handle result = mace_process_source("{ nope: }", ".", 0);
    assert(result != 0);

    char *message = mace_result_error(result);
    assert(message != 0);
    assert(mace_string_length(message) > 0);

    char *code = mace_result_error_code(result);
    if (code != 0) {
        assert(mace_string_length(code) > 0);
        mace_string_free(code);
    }
    mace_string_free(message);
    assert(mace_result_root(result) == 0);
    mace_result_free(result);
}

static void assert_request_lifecycle(void) {
    mace_handle request = mace_request_new(5000);
    assert(request != 0);

    mace_handle result = mace_process_source_with_request(
        request, "[output = 'data']\n{ tags: ['a', 7], }", ".", 0);
    assert(result != 0);
    assert(mace_result_error(result) == 0);

    mace_handle record = mace_result_root(result);
    assert_record_keys_are_sorted(record, 1);
    assert_nested_arrays(record);
    mace_result_free(result);
    mace_request_free(request);

    /* A freed request must not evaluate again. */
    mace_handle reused = mace_process_source_with_request(request, "[output = 'data']\n{ a: 1, }", ".", 0);
    assert(reused != 0);
    char *message = mace_result_error(reused);
    assert(message != 0);
    mace_string_free(message);
    mace_result_free(reused);
}

static void assert_workspace_boundary(void) {
    mace_handle result = mace_process_file("../../outside.mace", ".", 0);
    assert(result != 0);
    char *message = mace_result_error(result);
    assert(message != 0);
    mace_string_free(message);
    mace_result_free(result);
}

static void assert_missing_input_is_an_error(void) {
    mace_handle result = mace_process_source(0, ".", 0);
    assert(result != 0);
    char *message = mace_result_error(result);
    assert(message != 0);
    mace_string_free(message);
    mace_result_free(result);
}

int main(void) {
    assert(mace_abi_major() == 1);
    assert_evaluation("[output = 'data']\n{ count: 42, enabled: true, name: 'Ada', ratio: 1.0, }");
    assert_null_fields_are_dropped();
    assert_error_path_releases_its_message();
    assert_request_lifecycle();
    assert_workspace_boundary();
    assert_missing_input_is_an_error();
    return 0;
}
