package audience

import (
	"errors"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

// ErrEmptyUID is returned when an identifier carries no bytes.
var ErrEmptyUID = errors.New("audience: uid is empty")

// UID identifies one person to this platform. What the id stands for behind
// the scenes is not this platform's business, and neither is merging two of
// them into one person.
type UID string

func (u UID) validate() error {
	if len(u) == 0 {
		return ErrEmptyUID
	}
	return nil
}

// assignmentKey converts to the opaque key the experimentation layer takes.
//
// This conversion is where the knowledge lives. Experimentation has no idea
// what a key stands for, which is what lets it be opened to third parties;
// keeping alignment meaningful -- feeding it the same kind of thing twice --
// is therefore this layer's job, because this layer is the one that knows it
// is passing a uid.
func (u UID) assignmentKey() experimentation.AssignmentKey {
	return experimentation.AssignmentKey(u)
}
