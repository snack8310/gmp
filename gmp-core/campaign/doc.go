// Package campaign builds immutable activities, applies explicit rules, and
// returns decision, delivery-intent, execution, and receipt-transition values.
//
// It does not load audiences, save executions, deliver actions, publish events,
// retry work, or accept receipts atomically. An outer application owns those
// operations and decides which returned intents to commit or perform.
//
// The package sits above audience. It receives explicit records and assignment
// facts and has no runtime dependency on experimentation or external systems.
package campaign
