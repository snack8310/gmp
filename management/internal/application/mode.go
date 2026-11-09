package application

import "errors"

var ErrModeNotSet = errors.New("application: run mode was not set")

// RunMode chooses whether a runtime call records new assignment colorings.
type RunMode struct {
	rehearsal bool
	set       bool
}

func Live() RunMode      { return RunMode{set: true} }
func Rehearsal() RunMode { return RunMode{rehearsal: true, set: true} }

func (m RunMode) IsRehearsal() bool { return m.rehearsal }
func (m RunMode) IsSet() bool       { return m.set }
