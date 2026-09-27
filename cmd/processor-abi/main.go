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

// A handle registry maps opaque numeric handles to the Go state behind them.
// Go memory never crosses the C boundary: every exported call resolves its
// handle here under one lock, and strings are copied out into C memory.
type handleRegistry struct {
	mutex    sync.RWMutex
	next     uint64
	results  map[uint64]*nativeResult
	values   map[uint64]nativeValue
	requests map[uint64]nativeRequest
	limit    uint64
	overflow uint64
}

func newHandleRegistry(limit uint64) *handleRegistry {
	return &handleRegistry{
		results:  make(map[uint64]*nativeResult),
		values:   make(map[uint64]nativeValue),
		requests: make(map[uint64]nativeRequest),
		limit:    limit,
	}
}

func (registry *handleRegistry) live() uint64 {
	return uint64(len(registry.results) + len(registry.values) + len(registry.requests))
}

// addValue registers a value and its descendants. The caller holds the write lock.
func (registry *handleRegistry) addValue(value processor.Value, owner *nativeResult) uint64 {
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
			entry.children = append(entry.children, registry.addValue(value.Record[key], owner))
		}
	case processor.ValueArray:
		for _, child := range value.Array {
			entry.children = append(entry.children, registry.addValue(child, owner))
		}
	}

	registry.next++
	registry.values[registry.next] = entry
	owner.handles = append(owner.handles, registry.next)

	return registry.next
}

// addResult stores a finished evaluation. Once the live handle budget is spent it
// returns one shared result carrying ErrTooManyLiveHandles instead of growing, so
// an embedder that never frees results fails on every call rather than exhausting
// the host's memory.
func (registry *handleRegistry) addResult(output map[string]processor.Value, failure error) uint64 {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	if failure == nil && registry.live() >= registry.limit {
		return registry.overflowResult()
	}

	result := &nativeResult{failure: failure}
	if failure == nil {
		result.root = registry.addValue(processor.Value{Kind: processor.ValueRecord, Record: output}, result)
	}
	registry.next++
	registry.results[registry.next] = result

	return registry.next
}

// overflowResult reuses a single handle for every call made past the budget. The
// caller holds the write lock.
func (registry *handleRegistry) overflowResult() uint64 {
	if registry.overflow != 0 {
		return registry.overflow
	}

	registry.next++
	registry.overflow = registry.next
	registry.results[registry.overflow] = &nativeResult{failure: ErrTooManyLiveHandles}

	return registry.overflow
}

func (registry *handleRegistry) result(id uint64) (*nativeResult, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()

	owned, ok := registry.results[id]

	return owned, ok
}

func (registry *handleRegistry) resultRoot(id uint64) uint64 {
	owned, ok := registry.result(id)
	if !ok {
		return 0
	}

	return owned.root
}

func (registry *handleRegistry) freeResult(id uint64) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	owned, ok := registry.results[id]
	if !ok {
		return
	}
	if registry.overflow == id {
		registry.overflow = 0
	}
	for _, handle := range owned.handles {
		delete(registry.values, handle)
	}
	delete(registry.results, id)
}

func (registry *handleRegistry) value(id uint64) (nativeValue, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()

	entry, ok := registry.values[id]

	return entry, ok
}

func (registry *handleRegistry) addRequest(request nativeRequest) uint64 {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	registry.next++
	registry.requests[registry.next] = request

	return registry.next
}

func (registry *handleRegistry) request(id uint64) (nativeRequest, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()

	owned, ok := registry.requests[id]

	return owned, ok
}

func (registry *handleRegistry) cancelRequest(id uint64) {
	if owned, ok := registry.request(id); ok {
		owned.cancel()
	}
}

func (registry *handleRegistry) freeRequest(id uint64) {
	registry.mutex.Lock()
	owned, ok := registry.requests[id]
	delete(registry.requests, id)
	registry.mutex.Unlock()

	if ok {
		owned.cancel()
	}
}

// liveHandleLimit bounds the registry so a caller that leaks results or requests
// fails loudly instead of growing until the host runs out of memory. A single
// evaluation of a large configuration stays well inside this budget.
const liveHandleLimit = 1_000_000

// ErrTooManyLiveHandles reports that the registry's budget is exhausted, which
// means the embedder is not releasing results and requests.
var ErrTooManyLiveHandles = errors.New("too many live processor handles; free results and requests")

var handles = newHandleRegistry(liveHandleLimit)

func evaluate(operation context.Context, input *C.char, workspace *C.char, injection *C.char, file bool) C.uint64_t {
	if input == nil {
		return C.uint64_t(handles.addResult(nil, errors.New("missing source or path")))
	}

	root, err := os.Getwd()
	if err != nil {
		return C.uint64_t(handles.addResult(nil, err))
	}
	if workspace != nil {
		root = C.GoString(workspace)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return C.uint64_t(handles.addResult(nil, err))
	}

	var inputRecord map[string]processor.Value
	if injection != nil {
		var parseErr error
		inputRecord, parseErr = processor.ParseInputRecord(C.GoString(injection))
		if parseErr != nil {
			return C.uint64_t(handles.addResult(nil, parseErr))
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
	return C.uint64_t(handles.addResult(result.Output, err))
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
	return C.uint64_t(handles.addRequest(nativeRequest{operation: operation, cancel: cancel}))
}

//export mace_request_cancel
func mace_request_cancel(request C.uint64_t) {
	handles.cancelRequest(uint64(request))
}

//export mace_request_free
func mace_request_free(request C.uint64_t) {
	handles.freeRequest(uint64(request))
}

func processWithRequest(request C.uint64_t, input *C.char, workspace *C.char, injection *C.char, file bool) C.uint64_t {
	owned, ok := handles.request(uint64(request))
	if !ok {
		return C.uint64_t(handles.addResult(nil, errors.New("invalid processor request")))
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
	return C.uint64_t(handles.resultRoot(uint64(result)))
}

//export mace_result_error
func mace_result_error(result C.uint64_t) *C.char {
	owned, ok := handles.result(uint64(result))
	if ok && owned.failure != nil {
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
	if owned, ok := handles.result(uint64(result)); ok {
		_, code, _ := describeFailure(owned.failure)
		if code != "" {
			return C.CString(code)
		}
	}

	return nil
}

//export mace_result_error_kind
func mace_result_error_kind(result C.uint64_t) *C.char {
	if owned, ok := handles.result(uint64(result)); ok {
		kind, _, _ := describeFailure(owned.failure)
		if kind != "" {
			return C.CString(kind)
		}
	}

	return nil
}

//export mace_result_error_line
func mace_result_error_line(result C.uint64_t) C.uint32_t {
	return C.uint32_t(failureRange(uint64(result)).Start.Line)
}

//export mace_result_error_column
func mace_result_error_column(result C.uint64_t) C.uint32_t {
	return C.uint32_t(failureRange(uint64(result)).Start.Column)
}

//export mace_result_error_end_line
func mace_result_error_end_line(result C.uint64_t) C.uint32_t {
	return C.uint32_t(failureRange(uint64(result)).End.Line)
}

//export mace_result_error_end_column
func mace_result_error_end_column(result C.uint64_t) C.uint32_t {
	return C.uint32_t(failureRange(uint64(result)).End.Column)
}

func failureRange(id uint64) diagnostic.Range {
	owned, ok := handles.result(id)
	if !ok {
		return diagnostic.Range{}
	}

	_, _, span := describeFailure(owned.failure)

	return span
}

//export mace_result_free
func mace_result_free(result C.uint64_t) {
	handles.freeResult(uint64(result))
}

//export mace_value_kind
func mace_value_kind(id C.uint64_t) C.uint32_t {
	entry, _ := handles.value(uint64(id))
	return C.uint32_t(entry.kind)
}

//export mace_value_int
func mace_value_int(id C.uint64_t) C.int64_t {
	entry, _ := handles.value(uint64(id))
	return C.int64_t(entry.integer)
}

//export mace_value_float
func mace_value_float(id C.uint64_t) C.double {
	entry, _ := handles.value(uint64(id))
	return C.double(entry.decimal)
}

//export mace_value_boolean
func mace_value_boolean(id C.uint64_t) C.uint8_t {
	entry, _ := handles.value(uint64(id))
	if entry.boolean {
		return 1
	}
	return 0
}

//export mace_value_string
func mace_value_string(id C.uint64_t) *C.char {
	entry, _ := handles.value(uint64(id))
	return C.CString(entry.text)
}

//export mace_value_string_length
func mace_value_string_length(id C.uint64_t) C.uint64_t {
	entry, _ := handles.value(uint64(id))
	return C.uint64_t(len(entry.text))
}

//export mace_value_record_length
func mace_value_record_length(id C.uint64_t) C.uint64_t {
	entry, _ := handles.value(uint64(id))
	return C.uint64_t(len(entry.keys))
}

//export mace_value_record_key
func mace_value_record_key(id C.uint64_t, index C.uint64_t) *C.char {
	entry, _ := handles.value(uint64(id))
	if index >= C.uint64_t(len(entry.keys)) {
		return nil
	}
	return C.CString(entry.keys[int(index)])
}

//export mace_value_record_key_length
func mace_value_record_key_length(id C.uint64_t, index C.uint64_t) C.uint64_t {
	entry, _ := handles.value(uint64(id))
	if index >= C.uint64_t(len(entry.keys)) {
		return 0
	}
	return C.uint64_t(len(entry.keys[int(index)]))
}

//export mace_value_record_value
func mace_value_record_value(id C.uint64_t, index C.uint64_t) C.uint64_t {
	entry, _ := handles.value(uint64(id))
	if entry.kind != processor.ValueRecord || index >= C.uint64_t(len(entry.children)) {
		return 0
	}
	return C.uint64_t(entry.children[int(index)])
}

//export mace_value_array_length
func mace_value_array_length(id C.uint64_t) C.uint64_t {
	entry, _ := handles.value(uint64(id))
	if entry.kind != processor.ValueArray {
		return 0
	}
	return C.uint64_t(len(entry.children))
}

//export mace_value_array_item
func mace_value_array_item(id C.uint64_t, index C.uint64_t) C.uint64_t {
	entry, _ := handles.value(uint64(id))
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
