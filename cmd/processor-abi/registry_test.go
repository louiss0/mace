//go:build cgo

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/louiss0/mace/internal/processor"
)

func countedRecord() map[string]processor.Value {
	return map[string]processor.Value{"enabled": {Kind: processor.ValueBoolean, Boolean: true}}
}

func TestHandleRegistryRefusesNewResultsOnceTheLiveBudgetIsSpent(t *testing.T) {
	registry := newHandleRegistry(3)

	first := registry.addResult(countedRecord(), nil)
	spent := registry.live()
	if spent == 0 {
		t.Fatal("expected a registered result to consume live handles")
	}

	overflow := registry.addResult(countedRecord(), nil)
	owned, ok := registry.result(overflow)
	if !ok {
		t.Fatal("expected an overflow result handle to resolve")
	}
	if !errors.Is(owned.failure, ErrTooManyLiveHandles) {
		t.Fatalf("overflow failure = %v, want %v", owned.failure, ErrTooManyLiveHandles)
	}
	if first == overflow {
		t.Fatal("expected the overflow result to be a distinct handle")
	}
	if grew := registry.live(); grew > spent+1 {
		t.Fatalf("live handles grew from %d to %d past the budget", spent, grew)
	}
}

func TestHandleRegistryReusesTheOverflowResultAfterItIsFreed(t *testing.T) {
	registry := newHandleRegistry(1)
	registry.addResult(countedRecord(), nil)

	overflow := registry.addResult(countedRecord(), nil)
	if owned, ok := registry.result(overflow); !ok || !errors.Is(owned.failure, ErrTooManyLiveHandles) {
		t.Fatalf("expected the spent budget to produce an overflow result, got ok=%v failure=%v", ok, failureOf(owned))
	}

	registry.freeResult(overflow)

	reused := registry.addResult(countedRecord(), nil)
	owned, ok := registry.result(reused)
	if !ok || !errors.Is(owned.failure, ErrTooManyLiveHandles) {
		t.Fatalf("expected a resolvable overflow result after reuse, got ok=%v failure=%v", ok, failureOf(owned))
	}
}

func failureOf(result *nativeResult) error {
	if result == nil {
		return nil
	}

	return result.failure
}

func TestFreeingAResultReleasesEveryValueItOwns(t *testing.T) {
	registry := newHandleRegistry(liveHandleLimit)

	result := registry.addResult(countedRecord(), nil)
	root := registry.resultRoot(result)
	if live := registry.live(); live == 0 {
		t.Fatal("expected the result to own value handles")
	}

	registry.freeResult(result)

	if _, ok := registry.result(result); ok {
		t.Fatal("expected the result handle to be released")
	}
	if _, ok := registry.value(root); ok {
		t.Fatal("expected owned value handles to be released with the result")
	}
	if live := registry.live(); live != 0 {
		t.Fatalf("live handles = %d after freeing every result, want 0", live)
	}
}

func TestFreeingAnUnknownHandleIsHarmless(t *testing.T) {
	registry := newHandleRegistry(liveHandleLimit)

	registry.freeResult(9999)
	registry.freeRequest(9999)

	if _, ok := registry.result(9999); ok {
		t.Fatal("expected an unknown result handle to stay unresolved")
	}
	if _, ok := registry.request(9999); ok {
		t.Fatal("expected an unknown request handle to stay unresolved")
	}
}

func TestCancellingARequestStopsItsOperation(t *testing.T) {
	registry := newHandleRegistry(liveHandleLimit)
	operation, cancel := context.WithCancel(context.Background())
	defer cancel()

	handle := registry.addRequest(nativeRequest{operation: operation, cancel: cancel})
	registry.cancelRequest(handle)

	owned, ok := registry.request(handle)
	if !ok {
		t.Fatal("expected the request handle to resolve")
	}
	if !errors.Is(owned.operation.Err(), context.Canceled) {
		t.Fatalf("request error = %v, want %v", owned.operation.Err(), context.Canceled)
	}
}
