// Package coloringtest holds the behaviour every ColoringStore must show,
// written once and run against each implementation.
//
// Go interfaces are satisfied implicitly, so "this implementation matches the
// contract" is not something the compiler can check. A suite that runs against
// the in-memory store today and against a real store later is how that gap gets
// closed -- and it is only cheap to write while the in-memory store is the
// only implementation.
package coloringtest

import (
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

// NewStore builds a store that holds no records yet.
type NewStore func() experimentation.ColoringStore

// Run exercises the whole ColoringStore contract against the given
// implementation. Every new implementation is expected to call it.
func Run(t *testing.T, newStore NewStore) {
	t.Helper()
	t.Run("an unwritten key has no record", func(t *testing.T) {
		store := newStore()
		if _, found, err := store.Lookup("assignment", "key"); err != nil || found {
			t.Fatalf("empty store reported found=%v err=%v", found, err)
		}
	})

	t.Run("a written record is read back", func(t *testing.T) {
		store := newStore()
		want := experimentation.ColoringRecord{Bucket: mustBucket(t, 3), Algorithm: "fnv1a-v1"}
		if _, err := store.RecordFirst("assignment", "key", want); err != nil {
			t.Fatalf("recording: %v", err)
		}
		got, found, err := store.Lookup("assignment", "key")
		if err != nil || !found {
			t.Fatalf("reading back reported found=%v err=%v", found, err)
		}
		if got != want {
			t.Fatalf("read back %+v, wrote %+v", got, want)
		}
	})

	t.Run("the first record wins and later ones are told so", func(t *testing.T) {
		store := newStore()
		first := experimentation.ColoringRecord{Bucket: mustBucket(t, 2), Algorithm: "fnv1a-v1"}
		second := experimentation.ColoringRecord{Bucket: mustBucket(t, 7), Algorithm: "fnv1a-v1"}
		if _, err := store.RecordFirst("assignment", "key", first); err != nil {
			t.Fatalf("recording the first: %v", err)
		}
		inForce, err := store.RecordFirst("assignment", "key", second)
		if err != nil {
			t.Fatalf("recording the second: %v", err)
		}
		if inForce != first {
			t.Fatalf("the second write was told %+v is in force, expected %+v", inForce, first)
		}
		stored, _, err := store.Lookup("assignment", "key")
		if err != nil {
			t.Fatalf("reading back: %v", err)
		}
		if stored != first {
			t.Fatalf("the store holds %+v, the first write was %+v", stored, first)
		}
	})

	t.Run("records are kept per assignment and per key", func(t *testing.T) {
		store := newStore()
		here := experimentation.ColoringRecord{Bucket: mustBucket(t, 1), Algorithm: "fnv1a-v1"}
		there := experimentation.ColoringRecord{Bucket: mustBucket(t, 2), Algorithm: "fnv1a-v1"}
		if _, err := store.RecordFirst("assignment-a", "key", here); err != nil {
			t.Fatalf("recording under the first assignment: %v", err)
		}
		if _, err := store.RecordFirst("assignment-b", "key", there); err != nil {
			t.Fatalf("recording under the second assignment: %v", err)
		}
		if got, _, _ := store.Lookup("assignment-a", "key"); got != here {
			t.Fatalf("the first assignment holds %+v, expected %+v", got, here)
		}
		if got, _, _ := store.Lookup("assignment-b", "key"); got != there {
			t.Fatalf("the second assignment holds %+v, expected %+v", got, there)
		}
		if _, found, _ := store.Lookup("assignment-a", "another-key"); found {
			t.Fatal("a key that was never written came back with a record")
		}
	})

	t.Run("the recorded algorithm version is preserved", func(t *testing.T) {
		store := newStore()
		want := experimentation.ColoringRecord{Bucket: mustBucket(t, 1), Algorithm: "some-retired-version"}
		if _, err := store.RecordFirst("assignment", "key", want); err != nil {
			t.Fatalf("recording: %v", err)
		}
		got, _, err := store.Lookup("assignment", "key")
		if err != nil {
			t.Fatalf("reading back: %v", err)
		}
		if got.Algorithm != want.Algorithm {
			t.Fatalf("read back algorithm %q, wrote %q", got.Algorithm, want.Algorithm)
		}
	})
}

func mustBucket(t *testing.T, n int) experimentation.BucketNumber {
	t.Helper()
	b, err := experimentation.NewBucketNumber(n)
	if err != nil {
		t.Fatalf("building share %d: %v", n, err)
	}
	return b
}
