package internal

import (
	"github.com/samber/lo"
)

// Places too generic function

func NthOr[T any, N Integer](collection []T, nth N, defaultVal T) T {
	t, err := lo.Nth(collection, nth)
	if err != nil {
		return defaultVal
	}
	return t
}

func OneOf[T comparable](target T, candidates ...T) bool {
	return lo.Contains(candidates, target)
}

// Signed is a constraint that permits any signed integer type.
// If future releases of Go add new predeclared signed integer types,
// this constraint will be modified to include them.
type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// Unsigned is a constraint that permits any unsigned integer type.
// If future releases of Go add new predeclared unsigned integer types,
// this constraint will be modified to include them.
type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// Integer is a constraint that permits any integer type.
// If future releases of Go add new predeclared integer types,
// this constraint will be modified to include them.
type Integer interface {
	Signed | Unsigned
}
