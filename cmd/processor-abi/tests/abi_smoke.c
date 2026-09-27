#include "processor.h"
#include <assert.h>
#include <string.h>

int main(void) {
    assert(mace_abi_major() == 1);
    mace_handle result = mace_process_source("[output = 'data']\n{ name: 'Ada', count: 42, }", ".", 0);
    assert(result != 0);
    assert(mace_result_error(result) == 0);
    mace_handle record = mace_result_root(result);
    assert(mace_value_kind(record) == MACE_RECORD);
    assert(mace_value_record_length(record) == 2);
    char *key = mace_value_record_key(record, 0);
    assert(mace_value_record_key_length(record, 0) == 5);
    assert(memcmp(key, "count", 5) == 0);
    mace_string_free(key);
    mace_handle count = mace_value_record_value(record, 0);
    assert(mace_value_kind(count) == MACE_INT);
    assert(mace_value_int(count) == 42);
    mace_result_free(result);
    return 0;
}
