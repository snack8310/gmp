package audience_test

import (
	"errors"
	"testing"
	"time"

	"github.com/snack8310/gmp/gmp-core/audience"
)

func TestPureSourceRequiresIdentitySupplyAndCategory(t *testing.T) {
	tests := []struct {
		name string
		spec audience.SourceSpec
		want error
	}{
		{name: "missing id", spec: audience.SourceSpec{Supply: audience.SupplyPull, Category: audience.CategoryExternal}, want: audience.ErrMissingSourceID},
		{name: "unknown supply", spec: audience.SourceSpec{ID: "source", Category: audience.CategoryExternal}, want: audience.ErrUnknownSupply},
		{name: "unknown category", spec: audience.SourceSpec{ID: "source", Supply: audience.SupplyPull}, want: audience.ErrUnknownCategory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := audience.NewSource(test.spec); !errors.Is(err, test.want) {
				t.Fatalf("NewSource error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestPureDefinitionRejectsMissingIdentityAndNilCondition(t *testing.T) {
	if _, err := audience.NewDefinition(audience.DefinitionSpec{Source: "source"}); !errors.Is(err, audience.ErrMissingDefinitionID) {
		t.Fatalf("missing definition id error = %v, want ErrMissingDefinitionID", err)
	}
	if _, err := audience.NewDefinition(audience.DefinitionSpec{ID: "definition"}); !errors.Is(err, audience.ErrMissingSourceID) {
		t.Fatalf("missing source error = %v, want ErrMissingSourceID", err)
	}
	if _, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: "definition", Source: "source", Conditions: []audience.Condition{nil},
	}); !errors.Is(err, audience.ErrNilCondition) {
		t.Fatalf("nil condition error = %v, want ErrNilCondition", err)
	}
}

func TestPureElapsedConditionRequiresDeclaredSourceGranularity(t *testing.T) {
	undeclared, err := audience.NewSource(audience.SourceSpec{
		ID: "backflow-source", Supply: audience.SupplyPull, Category: audience.CategoryBackflow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := audience.ElapsedSince(undeclared, "message-sent", time.Hour); !errors.Is(err, audience.ErrGranularityNotDeclared) {
		t.Fatalf("undeclared granularity error = %v, want ErrGranularityNotDeclared", err)
	}

	declared, err := audience.NewSource(audience.SourceSpec{
		ID: "hourly-backflow", Supply: audience.SupplyPull, Category: audience.CategoryBackflow,
		BackflowGranularity: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := audience.ElapsedSince(declared, "message-sent", 10*time.Minute); !errors.Is(err, audience.ErrWindowShorterThanGranularity) {
		t.Fatalf("too-short window error = %v, want ErrWindowShorterThanGranularity", err)
	}
	if _, err := audience.ElapsedSince(declared, "message-sent", time.Hour); err != nil {
		t.Fatalf("a window equal to source granularity should build: %v", err)
	}
}
