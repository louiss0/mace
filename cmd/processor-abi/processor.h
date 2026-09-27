#ifndef MACE_PROCESSOR_H
#define MACE_PROCESSOR_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Result handles own all value handles. Free a result only after all reads end.
   Returned strings are UTF-8, copied into C memory, and require mace_string_free.
   Zero denotes a missing handle. A value handle is never independently freed. */
typedef uint64_t mace_handle;

enum mace_value_type {
    MACE_UNKNOWN = 0,
    /* Reserved. A null output field is dropped before the ABI, so no successful
       evaluation returns MACE_NULL. Bindings may keep decoding it defensively. */
    MACE_NULL = 1,
    MACE_STRING = 2,
    MACE_INT = 3,
    MACE_FLOAT = 4,
    MACE_HEX_INT = 5,
    MACE_HEX_FLOAT = 6,
    MACE_BOOLEAN = 7,
    MACE_ARRAY = 8,
    MACE_RECORD = 9
};

uint32_t mace_abi_major(void);
mace_handle mace_process_source(const char *source, const char *workspace, const char *input);
mace_handle mace_process_file(const char *path, const char *workspace, const char *input);
/* A request's timeout is in milliseconds; zero uses the 30-second default.
   Cancel/free from a different thread to stop an in-flight evaluation. */
mace_handle mace_request_new(uint32_t timeout_ms);
void mace_request_cancel(mace_handle request);
void mace_request_free(mace_handle request);
mace_handle mace_process_source_with_request(mace_handle request, const char *source, const char *workspace, const char *input);
mace_handle mace_process_file_with_request(mace_handle request, const char *path, const char *workspace, const char *input);
mace_handle mace_result_root(mace_handle result);
char *mace_result_error(mace_handle result);
char *mace_result_error_code(mace_handle result);
char *mace_result_error_kind(mace_handle result);
uint32_t mace_result_error_line(mace_handle result);
uint32_t mace_result_error_column(mace_handle result);
uint32_t mace_result_error_end_line(mace_handle result);
uint32_t mace_result_error_end_column(mace_handle result);
void mace_result_free(mace_handle result);
uint32_t mace_value_kind(mace_handle value);
int64_t mace_value_int(mace_handle value);
double mace_value_float(mace_handle value);
uint8_t mace_value_boolean(mace_handle value);
char *mace_value_string(mace_handle value);
uint64_t mace_value_string_length(mace_handle value);
uint64_t mace_value_record_length(mace_handle value);
char *mace_value_record_key(mace_handle value, uint64_t index);
uint64_t mace_value_record_key_length(mace_handle value, uint64_t index);
mace_handle mace_value_record_value(mace_handle value, uint64_t index);
uint64_t mace_value_array_length(mace_handle value);
mace_handle mace_value_array_item(mace_handle value, uint64_t index);
uint64_t mace_string_length(const char *value);
void mace_string_free(char *value);

#ifdef __cplusplus
}
#endif
#endif
