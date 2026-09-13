package experimentation

import (
	"errors"
	"fmt"
)

// ErrBucketOutOfRange is returned when a bucket number is built outside the
// shares a split actually has.
var ErrBucketOutOfRange = errors.New("experimentation: bucket number out of range")

// BucketNumber is the "第 N 份" of the domain documents, counted from one so
// that code and prose agree word for word.
//
// It is deliberately a struct rather than a defined integer type: a defined
// integer type can still index a slice, and an off-by-one between the
// one-based domain language and zero-based indexing would not fail loudly. The
// conversion happens in exactly one place, index().
type BucketNumber struct {
	n int
}

// NewBucketNumber builds the n-th share, counted from one.
func NewBucketNumber(n int) (BucketNumber, error) {
	if n < 1 {
		return BucketNumber{}, fmt.Errorf("%w: %d is below the first share", ErrBucketOutOfRange, n)
	}
	return BucketNumber{n: n}, nil
}

// Value returns the one-based share number, matching how the domain documents
// name it.
func (b BucketNumber) Value() int { return b.n }

// IsZero reports whether this is the zero value rather than a real share. The
// zero value is not share one; it means no share was assigned.
func (b BucketNumber) IsZero() bool { return b.n == 0 }

func (b BucketNumber) String() string {
	if b.IsZero() {
		return "no share"
	}
	return fmt.Sprintf("share %d", b.n)
}

// index converts to the zero-based offset used inside this package. It is the
// only place the two conventions meet.
func (b BucketNumber) index() int { return b.n - 1 }
