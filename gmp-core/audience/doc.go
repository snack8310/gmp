// Package audience answers two questions about one audience definition: who
// are these people, and does this one person count.
//
// Those are the only two things this layer offers. They are the same
// definition used in opposite directions -- enumeration may run offline and
// take minutes over millions, while a decision happens on the request path
// with an app waiting -- and they must agree. A definition that answers them
// differently breaks the model silently: the slot-serving path only ever asks
// about one id, so nothing would ever reconcile it against an enumeration.
//
// The layer sits above experimentation and imports nothing else. A share of an
// assignment is an ordinary condition here, the same kind of thing as "spent
// over a hundred" -- this package never buckets anyone itself.
//
// Nothing here reads a clock. Time enters as data: a source reports how long
// ago an event happened, and conditions compare that against a configured
// window. Waiting until a moment arrives belongs to whatever schedules the
// work, outside this model entirely.
package audience
