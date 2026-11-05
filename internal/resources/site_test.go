package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

// TestSiteChangesIgnoresUnconfiguredSettings: an optional setting that is not
// configured is null or unknown in the plan and must never cause a write.
func TestSiteChangesIgnoresUnconfiguredSettings(t *testing.T) {
	current := &client.SiteDetail{Name: "Default", TimeZone: "Europe/Brussels", NTPEnable: true, NTPServers: []string{"pool.ntp.org"}}
	plan := &siteModel{Name: types.StringValue("Default"), TimeZone: types.StringUnknown(), NTPEnabled: types.BoolNull(), NTPServers: types.ListNull(types.StringType)}
	if _, changed, diags := siteChanges(context.Background(), plan, current); changed || diags.HasError() {
		t.Errorf("siteChanges = changed %t, %v; want no change", changed, diags)
	}

	plan.TimeZone = types.StringValue("Europe/Amsterdam")
	plan.NTPServers = types.ListValueMust(types.StringType, nil)
	update, changed, _ := siteChanges(context.Background(), plan, current)
	if !changed || update.Name != nil || update.NTPEnable != nil || update.TimeZone == nil || *update.TimeZone != "Europe/Amsterdam" || update.NTPServers == nil || len(update.NTPServers) != 0 {
		t.Errorf("siteChanges = %+v, %t; want only the time zone and an empty server list", update, changed)
	}
}

func TestSiteSNMPChangesKeepsUnconfiguredSwitch(t *testing.T) {
	current := &client.SiteSNMP{V1V2CEnable: true, V3Enable: true}
	plan := &siteSNMPModel{V1V2CEnabled: types.BoolNull(), V3Enabled: types.BoolValue(false)}
	update, changed := siteSNMPChanges(plan, current)
	if !changed || update != (client.SiteSNMP{V1V2CEnable: true, V3Enable: false}) {
		t.Errorf("siteSNMPChanges = %+v, %t; want v1/v2c kept on, v3 off", update, changed)
	}
	plan.V3Enabled = types.BoolValue(true)
	if _, changed := siteSNMPChanges(plan, current); changed {
		t.Error("siteSNMPChanges reported a change for a matching setting")
	}
}
