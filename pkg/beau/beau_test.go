package beau

import (
	"fmt"
	"testing"
)

func TestSome(t *testing.T) {
	opt := Some(42)
	if !opt.IsSome() {
		t.Error("Some() should create IsSome() value")
	}
	if opt.IsNone() {
		t.Error("Some() should not create IsNone() value")
	}
	if opt.Unwrap() != 42 {
		t.Errorf("Expected 42, got %d", opt.Unwrap())
	}
}

func TestNone(t *testing.T) {
	opt := None[int]()
	if opt.IsSome() {
		t.Error("None() should not create IsSome() value")
	}
	if !opt.IsNone() {
		t.Error("None() should create IsNone() value")
	}
}

func TestUnwrap(t *testing.T) {
	some := Some(100)
	if some.Unwrap() != 100 {
		t.Errorf("Expected 100, got %d", some.Unwrap())
	}

	none := None[int]()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Unwrap() on None should panic")
		}
	}()
	none.Unwrap()
}

func TestUnwrapOr(t *testing.T) {
	some := Some(200)
	if some.UnwrapOr(999) != 200 {
		t.Errorf("Expected 200, got %d", some.UnwrapOr(999))
	}

	none := None[int]()
	if none.UnwrapOr(999) != 999 {
		t.Errorf("Expected 999, got %d", none.UnwrapOr(999))
	}
}

func TestUnwrapOrElse(t *testing.T) {
	some := Some(300)
	if some.UnwrapOrElse(func() int { return 888 }) != 300 {
		t.Errorf("Expected 300, got %d", some.UnwrapOrElse(func() int { return 888 }))
	}

	none := None[int]()
	if none.UnwrapOrElse(func() int { return 888 }) != 888 {
		t.Errorf("Expected 888, got %d", none.UnwrapOrElse(func() int { return 888 }))
	}
}

func TestMap(t *testing.T) {
	some := Some(5)
	mapped := Map(some, func(x int) string { return "value: " + string(rune(x+'0')) })
	if mapped.IsNone() {
		t.Error("Map on Some should return Some")
	}
	if mapped.Unwrap() != "value: 5" {
		t.Errorf("Expected 'value: 5', got '%s'", mapped.Unwrap())
	}

	none := None[int]()
	mappedNone := Map(none, func(x int) string { return "value: " + string(rune(x+'0')) })
	if !mappedNone.IsNone() {
		t.Error("Map on None should return None")
	}
}

func TestFilter(t *testing.T) {
	someEven := Some(4)
	filteredEven := Filter(someEven, func(x int) bool { return x%2 == 0 })
	if filteredEven.IsNone() {
		t.Error("Filter with true predicate on Some should return Some")
	}

	someOdd := Some(5)
	filteredOdd := Filter(someOdd, func(x int) bool { return x%2 == 0 })
	if !filteredOdd.IsNone() {
		t.Error("Filter with false predicate on Some should return None")
	}

	none := None[int]()
	filteredNone := Filter(none, func(x int) bool { return true })
	if !filteredNone.IsNone() {
		t.Error("Filter on None should return None")
	}
}

func TestAndThen(t *testing.T) {
	some := Some(10)
	result := AndThen(some, func(x int) Optional[string] {
		if x > 5 {
			return Some("big")
		}
		return None[string]()
	})
	if result.IsNone() {
		t.Error("AndThen with Some result should return Some")
	}
	if result.Unwrap() != "big" {
		t.Errorf("Expected 'big', got '%s'", result.Unwrap())
	}

	someSmall := Some(3)
	resultSmall := AndThen(someSmall, func(x int) Optional[string] {
		if x > 5 {
			return Some("big")
		}
		return None[string]()
	})
	if !resultSmall.IsNone() {
		t.Error("AndThen with None result should return None")
	}

	none := None[int]()
	resultNone := AndThen(none, func(x int) Optional[string] { return Some("anything") })
	if !resultNone.IsNone() {
		t.Error("AndThen on None should return None")
	}
}

func TestOrElse(t *testing.T) {
	some := Some(100)
	result := some.OrElse(func() Optional[int] { return Some(999) })
	if result.IsNone() {
		t.Error("OrElse on Some should return the Some value")
	}
	if result.Unwrap() != 100 {
		t.Errorf("Expected 100, got %d", result.Unwrap())
	}

	none := None[int]()
	resultNone := none.OrElse(func() Optional[int] { return Some(999) })
	if resultNone.IsNone() {
		t.Error("OrElse on None should return the alternative")
	}
	if resultNone.Unwrap() != 999 {
		t.Errorf("Expected 999, got %d", resultNone.Unwrap())
	}
}

func TestMust(t *testing.T) {
	// Test successful case
	result := Must(42, nil)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	// Test error case - should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must() should panic on error")
		} else {
			// Verify the panic value is the error
			err, ok := r.(error)
			if !ok {
				t.Error("Must() should panic with error type")
			}
			if err.Error() != "test error" {
				t.Errorf("Expected 'test error', got '%s'", err.Error())
			}
		}
	}()

	Must(0, fmt.Errorf("test error"))
}

func TestMFn(t *testing.T) {
	// Test successful case with no parameters
	testFunc0 := func() (int, error) {
		return 42, nil
	}
	result := MFn[int](testFunc0)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	// Test successful case with parameters
	testFunc2 := func(a, b int) (int, error) {
		return a + b, nil
	}
	result2 := MFn[int](testFunc2, 10, 20)
	if result2 != 30 {
		t.Errorf("Expected 30, got %d", result2)
	}

	// Test successful case with string parameter
	testFuncStr := func(s string) (string, error) {
		return "Hello " + s, nil
	}
	resultStr := MFn[string](testFuncStr, "World")
	if resultStr != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", resultStr)
	}

	// Test error case - should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("MFn() should panic on error")
		} else {
			// Verify the panic value is the error
			err, ok := r.(error)
			if !ok {
				t.Error("MFn() should panic with error type")
			}
			if err.Error() != "test error" {
				t.Errorf("Expected 'test error', got '%s'", err.Error())
			}
		}
	}()

	errorFunc := func() (int, error) {
		return 0, fmt.Errorf("test error")
	}
	MFn[int](errorFunc)
}

func TestMFnRobustness(t *testing.T) {
	// Test nil function
	defer func() {
		if r := recover(); r == nil {
			t.Error("MFn should panic with nil function")
		}
	}()
	MFn[int](nil)

	// Test non-function
	defer func() {
		if r := recover(); r == nil {
			t.Error("MFn should panic with non-function")
		}
	}()
	MFn[int]("not a function")

	// Test wrong number of arguments
	testFunc := func(a, b int) (int, error) { return a + b, nil }
	defer func() {
		if r := recover(); r == nil {
			t.Error("MFn should panic with wrong argument count")
		}
	}()
	MFn[int](testFunc, 1) // Missing second argument

	// Test wrong argument type
	defer func() {
		if r := recover(); r == nil {
			t.Error("MFn should panic with wrong argument type")
		}
	}()
	MFn[int](testFunc, 1, "wrong type")

	// Test function that doesn't return (T, error)
	badFunc := func() int { return 42 }
	defer func() {
		if r := recover(); r == nil {
			t.Error("MFn should panic with wrong return signature")
		}
	}()
	MFn[int](badFunc)
}
