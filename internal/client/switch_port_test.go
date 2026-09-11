package client

import (
	"context"
	"testing"
)

func TestSwitchPortReadModifyWrite(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	ctx := context.Background()
	port, err := c.FindSwitchPort(ctx, "site1", "d8:44:89:38:c6:c0", 2)
	if err != nil || port == nil {
		t.Fatalf("FindSwitchPort = %+v, %v; want physical port 2", port, err)
	}
	if err := c.UpdateSwitchPort(ctx, "site1", "d8:44:89:38:c6:c0", 2, SwitchPortConfig{
		Name: "Camera entrance", TagIDs: port.TagIDs, NativeNetworkID: port.NativeNetworkID,
		NetworkTagsSetting: port.NetworkTagsSetting, ProfileID: port.ProfileID,
		ProfileOverrideEnable: port.ProfileOverrideEnable, ProfileVLANOverrideEnable: port.ProfileVLANOverrideEnable,
		LinkSpeed: port.LinkSpeed, Duplex: port.Duplex,
	}); err != nil {
		t.Fatalf("UpdateSwitchPort: %v", err)
	}

	updated, err := c.FindSwitchPort(ctx, "site1", "D8-44-89-38-C6-C0", 2)
	if err != nil || updated == nil || updated.Name != "Camera entrance" {
		t.Fatalf("updated port = %+v, %v; want changed name", updated, err)
	}
	m.mu.Lock()
	gotPath := m.lastPatchPath
	m.mu.Unlock()
	wantPath := "/openapi/v1/omada1/sites/site1/switches/D8-44-89-38-C6-C0/ports/2"
	if gotPath != wantPath {
		t.Errorf("PATCH path = %q, want %q", gotPath, wantPath)
	}
}
