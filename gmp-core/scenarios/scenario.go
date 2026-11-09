package scenarios

// ID identifies one of the established cross-layer business regressions.
type ID string

const (
	NewCustomerFirstOrder ID = "new-customer-first-order"
	DoubleElevenWarmup    ID = "double-eleven-warmup"
	HomeBannerPersonalize ID = "home-banner-personalize"
	NewPlayRamp           ID = "new-play-ramp"
	CrossBorderRecall     ID = "cross-border-recall"
)
