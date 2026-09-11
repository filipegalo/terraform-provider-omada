package datasources

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSelectorHelpers(t *testing.T) {
	if boolCount(true, false, true) != 2 {
		t.Fatal("boolCount did not count configured selectors")
	}
	if configuredString(types.StringNull()) || configuredString(types.StringUnknown()) || configuredString(types.StringValue("")) {
		t.Fatal("null, unknown and empty strings must not be treated as selectors")
	}
	if !configuredString(types.StringValue("All")) {
		t.Fatal("non-empty string must be treated as a selector")
	}
	if got := selectedSiteID(types.StringNull(), "default-site"); got != "default-site" {
		t.Fatalf("selectedSiteID fallback = %q", got)
	}
}
