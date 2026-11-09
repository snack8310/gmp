// Package audience holds audience definitions and evaluates explicit records
// against them.
//
// It does not enumerate or fetch members. A caller supplies one source record
// and the assignment facts needed by share conditions; the package returns a
// match result and any placement values derivable from those facts.
//
// The package sits above experimentation. It never chooses a bucket or
// persists a coloring. Source and record values describe facts supplied by an
// outer application, and time-window conditions compare the ages carried by
// those records without reading a clock or scheduling work.
package audience
