// Package deliverytest holds the behaviour every Deliverer must show, written
// once and run against each implementation.
//
// The domain documents are explicit that a downstream being idempotent is a
// claim rather than a guarantee: one that says so and is not fails silently --
// a retry goes out, the person receives two of something, and nothing reports
// anything. So wiring a new downstream up is supposed to come with a way of
// demonstrating it, and this is that way.
package deliverytest

import (
	"testing"

	"github.com/snack8310/gmp/gmp-core/campaign"
)

// Subject is one implementation under test, plus a way to see what it actually
// did. Counting calls would not do: the whole question is whether two calls
// produced one effect.
type Subject struct {
	Deliverer campaign.Deliverer
	// Effects reports how many distinct deliveries really happened.
	Effects func() int
}

// New builds a subject that has handled nothing yet.
type New func() Subject

// Run exercises the whole Deliverer contract. Every new implementation is
// expected to call it.
func Run(t *testing.T, newSubject New) {
	t.Helper()

	t.Run("a fresh deliverer has done nothing", func(t *testing.T) {
		subject := newSubject()
		if got := subject.Effects(); got != 0 {
			t.Fatalf("a fresh deliverer reports %d effects", got)
		}
	})

	t.Run("one key delivered twice is one delivery", func(t *testing.T) {
		subject := newSubject()
		request := campaign.Request{Key: "the-same-key", UID: "u-1"}
		if _, err := subject.Deliverer.Deliver(request); err != nil {
			t.Fatalf("first delivery: %v", err)
		}
		if _, err := subject.Deliverer.Deliver(request); err != nil {
			t.Fatalf("second delivery under the same key: %v", err)
		}
		if got := subject.Effects(); got != 1 {
			t.Fatalf("two deliveries under one key produced %d effects, expected one", got)
		}
	})

	t.Run("two keys are two deliveries", func(t *testing.T) {
		subject := newSubject()
		for _, key := range []campaign.IdempotencyKey{"first", "second"} {
			if _, err := subject.Deliverer.Deliver(campaign.Request{Key: key, UID: "u-1"}); err != nil {
				t.Fatalf("delivering %q: %v", key, err)
			}
		}
		if got := subject.Effects(); got != 2 {
			t.Fatalf("two distinct keys produced %d effects, expected two", got)
		}
	})

	t.Run("the acknowledgement of a repeat matches the first", func(t *testing.T) {
		subject := newSubject()
		request := campaign.Request{Key: "repeat", UID: "u-1"}
		first, err := subject.Deliverer.Deliver(request)
		if err != nil {
			t.Fatalf("first delivery: %v", err)
		}
		again, err := subject.Deliverer.Deliver(request)
		if err != nil {
			t.Fatalf("second delivery: %v", err)
		}
		if again != first {
			t.Fatalf("the repeat was acknowledged as %+v, the first as %+v", again, first)
		}
	})
}
