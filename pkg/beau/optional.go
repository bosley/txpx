package beau

import (
	"fmt"
	"reflect"
)

func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func MFn[T any](f any, args ...any) T {
	if f == nil {
		panic("MFn: function cannot be nil")
	}

	fn := reflect.ValueOf(f)
	if !fn.IsValid() || fn.Kind() != reflect.Func {
		panic("MFn: first argument must be a valid function")
	}

	fnType := fn.Type()
	numIn := fnType.NumIn()
	if numIn != len(args) {
		panic(fmt.Sprintf("MFn: argument count mismatch; function expects %d args, got %d", numIn, len(args)))
	}

	// Pre-allocate and check parameter types
	reflectArgs := make([]reflect.Value, len(args))
	for i, arg := range args {
		if arg == nil {
			// check if parameter type accepts nil
			paramType := fnType.In(i)
			if paramType.Kind() != reflect.Ptr &&
				paramType.Kind() != reflect.Interface &&
				paramType.Kind() != reflect.Slice &&
				paramType.Kind() != reflect.Map &&
				paramType.Kind() != reflect.Chan &&
				paramType.Kind() != reflect.Func &&
				paramType.Kind() != reflect.Array {
				panic(fmt.Sprintf("MFn: argument %d cannot be nil (parameter type %s)", i, paramType.String()))
			}
			reflectArgs[i] = reflect.Zero(paramType)
		} else {
			argValue := reflect.ValueOf(arg)
			expectedType := fnType.In(i)
			if !argValue.Type().AssignableTo(expectedType) {
				panic(fmt.Sprintf("MFn: argument %d type mismatch; expected %s, got %s", i, expectedType.String(), argValue.Type().String()))
			}
			reflectArgs[i] = argValue
		}
	}

	// Validate return signature
	numOut := fnType.NumOut()
	if numOut != 2 {
		panic(fmt.Sprintf("MFn: function must return exactly 2 values, got %d", numOut))
	}

	errorType := fnType.Out(1)
	errorInterfaceType := reflect.TypeOf((*error)(nil)).Elem()
	if !errorType.AssignableTo(errorInterfaceType) {
		panic(fmt.Sprintf("MFn: second return value must be error, got %s", errorType.String()))
	}

	results := fn.Call(reflectArgs)

	errValue := results[1]
	if !errValue.IsNil() {
		if err, ok := errValue.Interface().(error); ok {
			panic(err)
		}
		panic("MFn: function returned non-nil second value that is not an error")
	}

	// Handle the result value
	resultValue := results[0]
	resultInterface := resultValue.Interface()

	// Handle nil results
	if resultInterface == nil {
		var zero T
		// For non-pointer types, returning zero value is fine
		// For pointer types, nil is acceptable
		return zero
	}

	if typedResult, ok := resultInterface.(T); ok {
		return typedResult
	}

	actualType := reflect.TypeOf(resultInterface)
	panic(fmt.Sprintf("MFn: return type mismatch; expected %T, got %s", *new(T), actualType.String()))
}

type Optional[T any] struct {
	v     T
	valid bool
}

func Some[T any](v T) Optional[T] {
	return Optional[T]{v: v, valid: true}
}

func None[T any]() Optional[T] {
	return Optional[T]{valid: false}
}

func (o *Optional[T]) Unwrap() T {
	if !o.valid {
		panic("called Unwrap() on a None value")
	}
	return o.v
}

func (o *Optional[T]) UnwrapOr(def T) T {
	if o.valid {
		return o.v
	}
	return def
}

func (o *Optional[T]) UnwrapOrElse(def func() T) T {
	if o.valid {
		return o.v
	}
	return def()
}

func (o *Optional[T]) IsSome() bool {
	return o.valid
}

func (o *Optional[T]) IsNone() bool {
	return !o.valid
}

func Map[T, U any](o Optional[T], f func(T) U) Optional[U] {
	if o.valid {
		return Some(f(o.v))
	}
	return None[U]()
}

func Filter[T any](o Optional[T], f func(T) bool) Optional[T] {
	if o.valid && f(o.v) {
		return o
	}
	return None[T]()
}

func AndThen[T, U any](o Optional[T], f func(T) Optional[U]) Optional[U] {
	if o.valid {
		return f(o.v)
	}
	return None[U]()
}

func (o *Optional[T]) OrElse(f func() Optional[T]) Optional[T] {
	if o.valid {
		return *o
	}
	return f()
}
