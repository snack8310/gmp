package campaign

import (
	"errors"
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
)

// ErrNoEventSink is returned when a runner is built without saying where its
// events go.
//
// A nil sink would mean events are dropped, and dropping them looks exactly
// like a second step whose conditions are too strict: nobody is reached and
// nothing fails. Saying "nowhere" has to be something you write down.
var ErrNoEventSink = errors.New("campaign: runner was not told where events go")

// EventName says what happened.
//
// The platform names its own events from the action and what became of it, so
// that a subscriber can ask for "the coupon went out" without knowing which
// campaign sent it. Inside one campaign the next treatment is chosen by an
// edge. An event name is what still crosses campaigns, experiments, and
// audience definitions built on what flowed back.
type EventName string

const (
	// eventDelivered is appended when a downstream took the delivery.
	eventDelivered = ":delivered"
	// eventArrived is appended when a receipt said it really arrived.
	eventArrived = ":arrived"
	// eventNotArrived is appended when a receipt said it did not.
	eventNotArrived = ":not-arrived"
)

// DeliveredEvent is the name this platform publishes when an action was taken
// by its downstream.
func DeliveredEvent(action ActionID) EventName { return EventName(string(action) + eventDelivered) }

// ArrivedEvent is the name published when a receipt confirms arrival.
func ArrivedEvent(action ActionID) EventName { return EventName(string(action) + eventArrived) }

// NotArrivedEvent is the name published when a receipt denies it.
func NotArrivedEvent(action ActionID) EventName { return EventName(string(action) + eventNotArrived) }

// Event is something this platform did, published so that anything else can
// react to it.
//
// Inside one campaign, a receipt edge names the next treatment. An event is
// how a result leaves the campaign: another campaign, an experiment, or an
// audience built over the backflow can subscribe without being a step on
// this flow.
type Event struct {
	Name EventName
	UID  audience.UID
	// Key ties the event to the execution that produced it, which is the same
	// key the delivery and the receipt carried.
	Key        IdempotencyKey
	Campaign   CampaignID
	Treatment  TreatmentID
	Action     ActionID
	Placements []audience.Placement
	// Attributes is whatever the channel said, unexamined. A receipt brings
	// back what that channel happens to report -- read, bounced, blocked -- and
	// there is deliberately no shared model for it, the same way there is none
	// for what an action takes as parameters.
	Attributes map[string]string
	// Outcome is the receipt result in the action's vocabulary, empty on a
	// delivery event and on a receipt whose action declared no contract.
	Outcome Outcome
}

// EventSink is where this platform's events go.
type EventSink interface {
	Publish(Event) error
}

// discardEvents drops everything.
type discardEvents struct{}

// DiscardEvents says, in so many words, that nothing subscribes.
//
// It exists so that having no subscribers is a decision someone wrote down
// rather than a field they forgot.
func DiscardEvents() EventSink { return discardEvents{} }

// Publish implements EventSink.
func (discardEvents) Publish(Event) error { return nil }

// Receipt is what a downstream eventually says about a delivery.
type Receipt struct {
	// Key is the same key the delivery carried.
	Key IdempotencyKey
	// Arrived says whether it got there.
	Arrived bool
	// ExternalRef is the downstream's own handle, if it gave one.
	ExternalRef string
	// Attributes is what this channel reports beyond arrival -- whether it was
	// read, why it bounced. One channel, one vocabulary.
	Attributes map[string]string
	// Outcome is the result in the action's vocabulary. It is not derived from
	// Arrived: the action's contract says what success and failure are, and a
	// message can arrive while the contract still calls the result a failure,
	// or name some further result entirely. Leave it empty when the action
	// declared no receipt contract. When a receipt edge leaves the treatment,
	// it has to be set.
	Outcome Outcome
}

func (r Receipt) validate() error {
	if r.Key == "" {
		return fmt.Errorf("%w: a receipt with no key", ErrNoSuchExecution)
	}
	return nil
}
