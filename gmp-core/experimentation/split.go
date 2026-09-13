package experimentation

import (
	"errors"
	"fmt"
)

var (
	// ErrNoShares is returned when a split carves the space into nothing.
	ErrNoShares = errors.New("experimentation: split has no shares")
	// ErrNonPositiveTotal is returned when the space being carved is empty.
	ErrNonPositiveTotal = errors.New("experimentation: split total is not positive")
	// ErrNegativeWeight is returned when a share claims a negative slice.
	ErrNegativeWeight = errors.New("experimentation: share weight is negative")
	// ErrWeightsDoNotSumToTotal is returned when the shares do not account for
	// the whole space. This is the "ratios that do not add up to the whole"
	// rejection the acceptance criteria asks for: the alternative would be to
	// normalise silently, which invents a rule the spec never stated.
	ErrWeightsDoNotSumToTotal = errors.New("experimentation: share weights do not sum to the total")
)

// Split is how one assignment carves its space: how many shares, and what each
// share's ratio is.
//
// The total is stated by the caller rather than assumed. Nothing here picks a
// default granularity -- a hard-coded 100 or 10000 would be exactly the kind of
// unprovable default the global constraints forbid. Stating it also turns "the
// ratios do not add up" into a rejection the caller can see, rather than a
// normalisation nobody asked for.
type Split struct {
	total   int
	weights []int
}

// NewSplit carves total into the given share weights, which must sum to total.
//
// Ratios that do not divide evenly are fine and exact: three equal shares of a
// total of three is a legal split, and no rounding happens anywhere.
func NewSplit(total int, weights []int) (Split, error) {
	if total <= 0 {
		return Split{}, fmt.Errorf("%w: %d", ErrNonPositiveTotal, total)
	}
	if len(weights) == 0 {
		return Split{}, ErrNoShares
	}
	sum := 0
	for i, w := range weights {
		if w < 0 {
			return Split{}, fmt.Errorf("%w: share %d has weight %d", ErrNegativeWeight, i+1, w)
		}
		sum += w
	}
	if sum != total {
		return Split{}, fmt.Errorf("%w: weights sum to %d, total is %d", ErrWeightsDoNotSumToTotal, sum, total)
	}
	owned := make([]int, len(weights))
	copy(owned, weights)
	return Split{total: total, weights: owned}, nil
}

// EqualSplit carves the space into shares of equal weight. It is the shape the
// rollout scenario uses -- a hundred shares, taken in gradually.
func EqualSplit(shares int) (Split, error) {
	if shares <= 0 {
		return Split{}, ErrNoShares
	}
	weights := make([]int, shares)
	for i := range weights {
		weights[i] = 1
	}
	return NewSplit(shares, weights)
}

// Shares reports how many shares this split has.
func (s Split) Shares() int { return len(s.weights) }

// Total reports the size of the space being carved.
func (s Split) Total() int { return s.total }

// Weight reports the weight of the given share.
func (s Split) Weight(b BucketNumber) (int, error) {
	if b.IsZero() || b.index() >= len(s.weights) {
		return 0, fmt.Errorf("%w: %s, split has %d", ErrBucketOutOfRange, b, len(s.weights))
	}
	return s.weights[b.index()], nil
}

// isZero reports whether this is an unset split rather than a carved one.
func (s Split) isZero() bool { return s.total == 0 && s.weights == nil }

// equals reports whether two splits carve the space identically. Alignment
// uses it: taking another assignment's salt while carving differently would
// not put the same key in the same share, and would not fail either.
func (s Split) equals(other Split) bool {
	if s.total != other.total || len(s.weights) != len(other.weights) {
		return false
	}
	for i := range s.weights {
		if s.weights[i] != other.weights[i] {
			return false
		}
	}
	return true
}

// bucketFor maps a position inside the total space to the share that owns it.
// Shares with zero weight own nothing and can never be returned.
func (s Split) bucketFor(position int) (BucketNumber, error) {
	if position < 0 || position >= s.total {
		return BucketNumber{}, fmt.Errorf("%w: position %d outside total %d", ErrBucketOutOfRange, position, s.total)
	}
	cumulative := 0
	for i, w := range s.weights {
		cumulative += w
		if position < cumulative {
			return NewBucketNumber(i + 1)
		}
	}
	// Unreachable while weights sum to total, which the constructor enforces.
	return BucketNumber{}, fmt.Errorf("%w: position %d found no share", ErrBucketOutOfRange, position)
}
