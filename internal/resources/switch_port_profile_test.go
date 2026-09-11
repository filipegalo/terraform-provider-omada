package resources

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSwitchPortProfileConfigKeepsExplicitEmptyTaggedSet(t *testing.T) {
	model := switchPortProfileModel{
		Name:             types.StringValue("Access"),
		TaggedNetworkIDs: types.SetValueMust(types.StringType, nil),
	}
	cfg := switchPortProfileConfig(model)
	if cfg.TaggedNetworkIDs == nil || len(*cfg.TaggedNetworkIDs) != 0 {
		t.Fatalf("tagged network IDs = %#v; want an explicit empty list", cfg.TaggedNetworkIDs)
	}
}
