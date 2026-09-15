package scenarios

import (
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

// SourceID is the source the fixture population comes from.
const SourceID = audience.SourceID("app-profile")

// BannerSlot is the slot the scenarios contend for.
const BannerSlot = campaign.SlotID("app-home-banner")

// Stack is the three layers wired together over in-memory implementations.
//
// It is ordinary code rather than a test helper so that the demo and the
// regression cases assemble the model exactly the same way. Two assemblies
// that drift apart would mean the thing being demonstrated is not the thing
// being tested.
type Stack struct {
	Experiments *experimentation.Service
	Colouring   *experimentation.MemoryColoringStore
	Audiences   *audience.Service
	Source      audience.Source
	People      []audience.Record
}

// NewStack wires the layers and fills a population.
func NewStack(population int) (*Stack, error) {
	if population <= 0 {
		return nil, fmt.Errorf("scenarios: population must be positive, got %d", population)
	}
	registry, err := experimentation.NewAlgorithmRegistry(experimentation.FNV1aV1{})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the algorithm registry: %w", err)
	}
	colouring := experimentation.NewMemoryColoringStore()
	experiments, err := experimentation.NewService(registry, colouring)
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the experimentation service: %w", err)
	}

	source, err := audience.RegisterSource(audience.SourceSpec{
		ID:       SourceID,
		Supply:   audience.SupplyPull,
		Category: audience.CategoryExternal,
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: registering the source: %w", err)
	}

	people := make([]audience.Record, population)
	for i := range people {
		tier := "other"
		switch i % 3 {
		case 0:
			tier = "new"
		case 1:
			tier = "returning"
		}
		attributes := map[string]string{"tier": tier}
		if i%5 == 0 {
			attributes["added-to-cart"] = "yes"
		}
		people[i] = audience.Record{
			UID:        audience.UID(fmt.Sprintf("u-%05d", i)),
			Attributes: attributes,
		}
	}

	audiences := audience.NewService(experiments)
	if err := audiences.Register(source, audience.NewMemoryFeed(people...)); err != nil {
		return nil, fmt.Errorf("scenarios: registering the feed: %w", err)
	}

	return &Stack{
		Experiments: experiments,
		Colouring:   colouring,
		Audiences:   audiences,
		Source:      source,
		People:      people,
	}, nil
}

// NewCampaignService builds a campaign service with the banner slot in place.
//
// Each scenario gets its own: campaigns are immutable and a service refuses to
// hold one twice, so comparing two configurations means two services.
func (s *Stack) NewCampaignService() (*campaign.Service, error) {
	service := campaign.NewService(s.Audiences)
	slot, err := campaign.RegisterSlot(BannerSlot)
	if err != nil {
		return nil, fmt.Errorf("scenarios: registering the slot: %w", err)
	}
	if err := service.RegisterSlot(slot); err != nil {
		return nil, fmt.Errorf("scenarios: putting the slot into service: %w", err)
	}
	return service, nil
}

// Definition builds an audience definition over the fixture source.
func (s *Stack) Definition(id string, conditions ...audience.Condition) (audience.Definition, error) {
	return audience.NewDefinition(audience.DefinitionSpec{
		ID:         audience.DefinitionID(id),
		Source:     SourceID,
		Conditions: conditions,
	})
}

// EqualAssignment builds a coloured or uncoloured assignment carving equal
// shares.
func EqualAssignment(id string, shares int, coloured bool) (experimentation.Definition, error) {
	split, err := experimentation.EqualSplit(shares)
	if err != nil {
		return experimentation.Definition{}, fmt.Errorf("scenarios: carving %d shares: %w", shares, err)
	}
	return experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID:        experimentation.AssignmentID(id),
		Split:     split,
		Salt:      experimentation.Salt(id),
		Algorithm: "fnv1a-v1",
		Colored:   coloured,
	})
}

// Share is the n-th share, counted from one as everywhere else.
func Share(n int) (experimentation.BucketNumber, error) {
	return experimentation.NewBucketNumber(n)
}
