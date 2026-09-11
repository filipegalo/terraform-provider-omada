package client

import (
	"context"
	"testing"
)

func TestSwitchPortProfileReadModifyWritePreservesUnmanagedFields(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)
	ctx := context.Background()

	native := "network-management"
	tagged := []string{"network-home", "network-iot"}
	custom := int64(2)
	enabled := true
	id, err := c.CreateSwitchPortProfile(ctx, "site1", SwitchPortProfileConfig{
		Name: "AP-Trunk", NativeNetworkID: &native, TaggedNetworkIDs: &tagged,
		VLANConfigEnable: &enabled, NetworkTagsSetting: &custom,
	})
	if err != nil {
		t.Fatalf("CreateSwitchPortProfile: %v", err)
	}

	tagged = []string{"network-home", "network-iot", "network-guest"}
	if err := c.UpdateSwitchPortProfile(ctx, "site1", id, SwitchPortProfileConfig{
		Name: "AP-Trunk", NativeNetworkID: &native, TaggedNetworkIDs: &tagged,
		VLANConfigEnable: &enabled, NetworkTagsSetting: &custom,
	}); err != nil {
		t.Fatalf("UpdateSwitchPortProfile: %v", err)
	}

	m.mu.Lock()
	updated := m.profiles[id]
	m.mu.Unlock()
	if updated["poe"] != float64(2) || updated["prohibitModify"] != false {
		t.Fatalf("unmanaged top-level fields were not preserved: %#v", updated)
	}
	stp, ok := updated["spanningTreeSetting"].(map[string]any)
	if !ok || stp["priority"] != float64(128) || stp["instances"] == nil {
		t.Fatalf("unmanaged spanning-tree settings were not preserved: %#v", updated["spanningTreeSetting"])
	}
	profile, err := c.FindSwitchPortProfile(ctx, "site1", id)
	if err != nil || profile == nil || len(profile.TaggedNetworkIDs) != 3 {
		t.Fatalf("updated profile = %#v, %v; want three tagged VLANs", profile, err)
	}
}
