package management_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
	"github.com/snack8310/gmp/management/internal/management"
)

func TestMySQLColoringRecordFirstConcurrentWinner(t *testing.T) {
	config := isolatedMySQLIntegrationConfig(t)
	logger := slog.New(slog.NewJSONHandler(ioDiscard{}, nil))
	stores := make([]*management.MySQLStore, 2)
	for index := range stores {
		store, err := management.OpenMySQLStore(context.Background(), config, logger)
		if err != nil {
			t.Fatalf("opening MySQL store %d: %v", index, err)
		}
		stores[index] = store
		defer store.Close()
	}

	firstBucket, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	secondBucket, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	records := []experimentation.ColoringRecord{
		{Bucket: firstBucket, Algorithm: "fnv1a-v1"},
		{Bucket: secondBucket, Algorithm: "fnv1a-v1"},
	}
	start := make(chan struct{})
	results := make(chan experimentation.ColoringRecord, len(stores))
	errors := make(chan error, len(stores))
	var group sync.WaitGroup
	for index := range stores {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			winner, err := stores[index].RecordColoringFirst(context.Background(), "assignment-concurrent", "key-concurrent", records[index])
			if err != nil {
				errors <- err
				return
			}
			results <- winner
		}(index)
	}
	close(start)
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatalf("recording concurrent coloring: %v", err)
	}

	var winner experimentation.ColoringRecord
	count := 0
	for result := range results {
		if count > 0 && result != winner {
			t.Fatalf("concurrent callers saw different winners: %#v and %#v", winner, result)
		}
		winner = result
		count++
	}
	if count != len(stores) || (winner != records[0] && winner != records[1]) {
		t.Fatalf("winner = %#v from %d calls, want one database winner from both calls", winner, count)
	}

	otherAssignment, err := stores[0].RecordColoringFirst(context.Background(), "assignment-other", "key-concurrent", records[0])
	if err != nil || otherAssignment != records[0] {
		t.Fatalf("different assignment coloring = %#v, err = %v", otherAssignment, err)
	}
	otherKey, err := stores[0].RecordColoringFirst(context.Background(), "assignment-concurrent", "key-other", records[1])
	if err != nil || otherKey != records[1] {
		t.Fatalf("different key coloring = %#v, err = %v", otherKey, err)
	}

	stored, found, err := stores[1].LookupColoring(context.Background(), "assignment-concurrent", "key-concurrent")
	if err != nil || !found || stored != winner {
		t.Fatalf("stored winner = %#v (found=%v err=%v), want %#v", stored, found, err, winner)
	}
}
