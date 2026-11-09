package audience

import "fmt"

// Placement records which share of which assignment a person landed in.
type Placement struct {
	Assignment string
	Share      int
}

func (p Placement) String() string {
	return fmt.Sprintf("assignment %q share %d", p.Assignment, p.Share)
}
