package campaign

import (
	"fmt"
	"sync"
)

// MemoryDeliverer is an in-memory Deliverer: the first implementation,
// deliberately not wired to anything real.
//
// It is idempotent by key, which is what every deliverer is required to be.
// Being able to say so here is not the same as a real downstream being so --
// see DelivererContract, which exists because "we are idempotent" is a claim.
type MemoryDeliverer struct {
	mu       sync.Mutex
	handled  map[IdempotencyKey]Ack
	calls    []Request
	failures map[IdempotencyKey]int
	nextRef  int
}

// NewMemoryDeliverer builds a deliverer that has handled nothing.
func NewMemoryDeliverer() *MemoryDeliverer {
	return &MemoryDeliverer{
		handled:  make(map[IdempotencyKey]Ack),
		failures: make(map[IdempotencyKey]int),
	}
}

// FailNext makes the next few deliveries of this key fail, so that retrying can
// be exercised without anything real being unreliable.
func (d *MemoryDeliverer) FailNext(key IdempotencyKey, times int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.failures[key] = times
}

// Deliver implements Deliverer.
func (d *MemoryDeliverer) Deliver(request Request) (Ack, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, request)
	if remaining := d.failures[request.Key]; remaining > 0 {
		d.failures[request.Key] = remaining - 1
		return Ack{}, fmt.Errorf("memory deliverer: refusing %q on purpose", request.Key)
	}
	if ack, already := d.handled[request.Key]; already {
		return ack, nil
	}
	d.nextRef++
	ack := Ack{ExternalRef: fmt.Sprintf("memory-%d", d.nextRef)}
	d.handled[request.Key] = ack
	return ack, nil
}

// Effects reports how many distinct deliveries actually happened, as opposed to
// how many times Deliver was called.
func (d *MemoryDeliverer) Effects() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.handled)
}

// Calls reports every call, retries included, in order.
func (d *MemoryDeliverer) Calls() []Request {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Request, len(d.calls))
	copy(out, d.calls)
	return out
}
