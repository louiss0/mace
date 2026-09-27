//go:build cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/louiss0/mace/internal/diagnostic"
	"github.com/louiss0/mace/internal/processor"
)

// An opaque numeric handle keeps Go pointers out of C memory. A result owns
// every descendant; callers must not read descendants after freeing it.
type nativeValue struct {
	kind     processor.ValueKind
	text     string
	integer  int64
	decimal  float64
	boolean  bool
	keys     []string
	children []uint64
}

type nativeResult struct {
	root    uint64
	handles []uint64
	failure error
}

type nativeRequest struct {
	operation context.Context
	cancel    context.CancelFunc
}

var handles = struct {
	sync.RWMutex
	next     uint64
	results  map[uint64]*nativeResult
	values   map[uint64]nativeValue
	requests map[uint64]nativeRequest
}{results: make(map[uint64]*nativeResult), values: make(map[uint64]nativeValue), requests: make(map[uint64]nativeRequest)}

func registerValue(value processor.Value, owner *nativeResult) uint64 {
	entry := nativeValue{
		kind:    value.Kind,
		text:    value.String,
		integer: value.Int,
		decimal: value.Float,
		boolean: value.Boolean,
	}
	if value.Kind == processor.ValueHexInt || value.Kind == processor.ValueHexFloat {
		entry.text, _ = processor.FormatScalarValue(value)
	}

	switch value.Kind {
	case processor.ValueRecord:
		entry.keys = make([]string, 0, len(value.Record))
		for key := range value.Record {
			entry.keys = append(entry.keys, key)
		}
		slices.Sort(entry.keys)
		for _, key := range entry.keys {
			entry.children = append(entry.children, registerValue(value.Record[key], owner))
		}
	case processor.ValueArray:
		for _, child := range value.Array {
			entry.children = append(entry.children, registerValue(child, owner))
		}
	}

	handles.next++
	id := handles.next
	handles.values[id] = entry
	owner.handles = append(owner.handles, id)
	return id
}

func registerResult(output map[string]processor.Value, failure error) C.uint64_t {
	handles.Lock()
	defer handles.Unlock()

	result := &nativeResult{failure: failure}
	if failure == nil {
		result.root = registerValue(processor.Value{Kind: processor.ValueRecord, Record: output}, result)
	}
	handles.next++
	id := handles.next
	handles.results[id] = result
	return C.uint64_t(id)
}

func evaluate(operation context.Context, input *C.char, workspace *C.char, injection *C.char, file bool) C.uint64_t {
	if input == nil {
		return registerResult(nil, errors.New("missing source or path"))
	}

	root, err := os.Getwd()
	if err != nil {
		return registerResult(nil, err)
	}
	if workspace != nil {
		root = C.GoString(workspace)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return registerResult(nil, err)
	}

	var inputRecord map[string]processor.Value
	if injection != nil {
		var parseErr error
		inputRecord, parseErr = processor.ParseInputRecord(C.GoString(injection))
		if parseErr != nil {
			return registerResult(nil, parseErr)
		}
	}
	instance := processor.NewWithContext(operation, inputRecord)

	var result processor.Result
	if file {
		path := C.GoString(input)
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		result, err = instance.ProcessFileInDir(path, root)
	} else {
		result, err = instance.ProcessInDir(C.GoString(input), root)
	}
	return registerResult(result.Output, err)
}

//export mace_abi_major
func mace_abi_major() C.uint32_t {
	return 1
}

//export mace_process_source
func mace_process_source(source *C.char, workspace *C.char, injection *C.char) C.uint64_t {
	operation, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return evaluate(operation, source, workspace, injection, false)
}

//export mace_process_file
func mace_process_file(path *C.char, workspace *C.char, injection *C.char) C.uint64_t {
	operation, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return evaluate(operation, path, workspace, injection, true)
}

//export mace_request_new
func mace_request_new(timeout C.uint32_t) C.uint64_t {
	if timeout == 0 {
		timeout = 30_000
	}
	operation, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
	handles.Lock()
	defer handles.Unlock()
	handles.next++
	id := handles.next
	handles.requests[id] = nativeRequest{operation: operation, cancel: cancel}
	return C.uint64_t(id)
}

//export mace_request_cancel
func mace_request_cancel(request C.uint64_t) {
	handles.RLock()
	owned, ok := handles.requests[uint64(request)]
	handles.RUnlock()
	if ok {
		owned.cancel()
	}
}

//export mace_request_free
func mace_request_free(request C.uint64_t) {
	handles.Lock()
	owned, ok := handles.requests[uint64(request)]
	delete(handles.requests, uint64(request))
	handles.Unlock()
	if ok {
		owned.cancel()
	}
}

func processWithRequest(request C.uint64_t, input *C.char, workspace *C.char, injection *C.char, file bool) C.uint64_t {
	handles.RLock()
	owned, ok := handles.requests[uint64(request)]
	handles.RUnlock()
	if !ok {
		return registerResult(nil, errors.New("invalid processor request"))
	}
	return evaluate(owned.operation, input, workspace, injection, file)
}

//export mace_process_source_with_request
func mace_process_source_with_request(request C.uint64_t, source *C.char, workspace *C.char, injection *C.char) C.uint64_t {
	return processWithRequest(request, source, workspace, injection, false)
}

//export mace_process_file_with_request
func mace_process_file_with_request(request C.uint64_t, path *C.char, workspace *C.char, injection *C.char) C.uint64_t {
	return processWithRequest(request, path, workspace, injection, true)
}

//export mace_result_root
func mace_result_root(result C.uint64_t) C.uint64_t {
	handles.RLock()
	defer handles.RUnlock()
	if owned := handles.results[uint64(result)]; owned != nil {
		return C.uint64_t(owned.root)
	}
	return 0
}

//export mace_result_error
func mace_result_error(result C.uint64_t) *C.char {
	handles.RLock()
	defer handles.RUnlock()
	if owned := handles.results[uint64(result)]; owned != nil && owned.failure != nil {
		return C.CString(owned.failure.Error())
	}
	return nil
}

func describeFailure(failure error) (string, string, diagnostic.Range) {
	if errors.Is(failure, context.DeadlineExceeded) {
		return "timeout", "mace.runtime.timeout", diagnostic.Range{}
	}
	if errors.Is(failure, context.Canceled) {
		return "cancelled", "mace.runtime.cancelled", diagnostic.Range{}
	}
	var processorError processor.DiagnosticError
	if errors.As(failure, &processorError) {
		return string(processorError.Kind), string(processorError.Code), processorError.Range
	}
	if other, ok := diagnostic.As(failure); ok {
		return other.Kind, string(other.Code), other.Range
	}
	return "", "", diagnostic.Range{}
}

//export mace_result_error_code
func mace_result_error_code(result C.uint64_t) *C.char {
	handles.RLock()
	defer handles.RUnlock()
	if owned := handles.results[uint64(result)]; owned != nil {
		_, code, _ := describeFailure(owned.failure)
		if code != "" {
			return C.CString(code)
		}
	}
	return nil
}

//export mace_result_error_kind
func mace_result_error_kind(result C.uint64_t) *C.char {
	handles.RLock()
	defer handles.RUnlock()
	if owned := handles.results[uint64(result)]; owned != nil {
		kind, _, _ := describeFailure(owned.failure)
		if kind != "" {
			return C.CString(kind)
		}
	}
	return nil
}

//export mace_result_error_line
func mace_result_error_line(result C.uint64_t) C.uint32_t {
	handles.RLock()
	defer handles.RUnlock()
	if owned := handles.results[uint64(result)]; owned != nil {
		_, _, span := describeFailure(owned.failure)
		return C.uint32_t(span.Start.Line)
	}
	return 0
}

//export mace_result_error_column
func mace_result_error_column(result C.uint64_t) C.uint32_t {
	handles.RLock()
	defer handles.RUnlock()
	if owned := handles.results[uint64(result)]; owned != nil {
		_, _, span := describeFailure(owned.failure)
		return C.uint32_t(span.Start.Column)
	}
	return 0
}

//export mace_result_error_end_line
func mace_result_error_end_line(result C.uint64_t) C.uint32_t {
	handles.RLock()
	defer handles.RUnlock()
	if owned := handles.results[uint64(result)]; owned != nil {
		_, _, span := describeFailure(owned.failure)
		return C.uint32_t(span.End.Line)
	}
	return 0
}

//export mace_result_error_end_column
func mace_result_error_end_column(result C.uint64_t) C.uint32_t {
	handles.RLock()
	defer handles.RUnlock()
	if owned := handles.results[uint64(result)]; owned != nil {
		_, _, span := describeFailure(owned.failure)
		return C.uint32_t(span.End.Column)
	}
	return 0
}

//export mace_result_free
func mace_result_free(result C.uint64_t) {
	handles.Lock()
	defer handles.Unlock()
	if owned := handles.results[uint64(result)]; owned != nil {
		for _, id := range owned.handles {
			delete(handles.values, id)
		}
		delete(handles.results, uint64(result))
	}
}

func findValue(id C.uint64_t) nativeValue {
	return handles.values[uint64(id)]
}

//export mace_value_kind
func mace_value_kind(id C.uint64_t) C.uint32_t {
	handles.RLock()
	defer handles.RUnlock()
	return C.uint32_t(findValue(id).kind)
}

//export mace_value_int
func mace_value_int(id C.uint64_t) C.int64_t {
	handles.RLock()
	defer handles.RUnlock()
	return C.int64_t(findValue(id).integer)
}

//export mace_value_float
func mace_value_float(id C.uint64_t) C.double {
	handles.RLock()
	defer handles.RUnlock()
	return C.double(findValue(id).decimal)
}

//export mace_value_boolean
func mace_value_boolean(id C.uint64_t) C.uint8_t {
	handles.RLock()
	defer handles.RUnlock()
	if findValue(id).boolean {
		return 1
	}
	return 0
}

//export mace_value_string
func mace_value_string(id C.uint64_t) *C.char {
	handles.RLock()
	defer handles.RUnlock()
	if id == 0 {
		return nil
	}
	return C.CString(findValue(id).text)
}

//export mace_value_string_length
func mace_value_string_length(id C.uint64_t) C.uint64_t {
	handles.RLock()
	defer handles.RUnlock()
	return C.uint64_t(len(findValue(id).text))
}

//export mace_value_record_length
func mace_value_record_length(id C.uint64_t) C.uint64_t {
	handles.RLock()
	defer handles.RUnlock()
	return C.uint64_t(len(findValue(id).keys))
}

//export mace_value_record_key
func mace_value_record_key(id C.uint64_t, index C.uint64_t) *C.char {
	handles.RLock()
	defer handles.RUnlock()
	entry := findValue(id)
	if index >= C.uint64_t(len(entry.keys)) {
		return nil
	}
	return C.CString(entry.keys[int(index)])
}

//export mace_value_record_key_length
func mace_value_record_key_length(id C.uint64_t, index C.uint64_t) C.uint64_t {
	handles.RLock()
	defer handles.RUnlock()
	entry := findValue(id)
	if index >= C.uint64_t(len(entry.keys)) {
		return 0
	}
	return C.uint64_t(len(entry.keys[int(index)]))
}

//export mace_value_record_value
func mace_value_record_value(id C.uint64_t, index C.uint64_t) C.uint64_t {
	handles.RLock()
	defer handles.RUnlock()
	entry := findValue(id)
	if entry.kind != processor.ValueRecord || index >= C.uint64_t(len(entry.children)) {
		return 0
	}
	return C.uint64_t(entry.children[int(index)])
}

//export mace_value_array_length
func mace_value_array_length(id C.uint64_t) C.uint64_t {
	handles.RLock()
	defer handles.RUnlock()
	entry := findValue(id)
	if entry.kind != processor.ValueArray {
		return 0
	}
	return C.uint64_t(len(entry.children))
}

//export mace_value_array_item
func mace_value_array_item(id C.uint64_t, index C.uint64_t) C.uint64_t {
	handles.RLock()
	defer handles.RUnlock()
	entry := findValue(id)
	if entry.kind != processor.ValueArray || index >= C.uint64_t(len(entry.children)) {
		return 0
	}
	return C.uint64_t(entry.children[int(index)])
}

//export mace_string_length
func mace_string_length(value *C.char) C.uint64_t {
	if value == nil {
		return 0
	}
	return C.uint64_t(C.strlen(value))
}

//export mace_string_free
func mace_string_free(value *C.char) {
	C.free(unsafe.Pointer(value))
}

func main() {}
