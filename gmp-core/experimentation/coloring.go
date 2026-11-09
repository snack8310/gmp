package experimentation

// ColoringRecord is a persisted assignment fact. Storage and first-write
// arbitration belong to the application layer.
type ColoringRecord struct {
	Bucket    BucketNumber
	Algorithm AlgorithmVersion
}
