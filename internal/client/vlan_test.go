package client

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestVLANCRUD(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	ctx := context.Background()
	cfg := VLANConfig{Name: "IoT", DeviceMAC: "AA-BB-CC-DD-EE-FF", DeviceType: 1, VLANType: 0, VLANID: 20, GatewaySubnet: "192.0.2.1/24", DHCPEnabled: true, DHCPRangeStart: "192.0.2.100", DHCPRangeEnd: "192.0.2.199", DHCPDNSMode: "auto", DHCPLeaseTime: 1440}
	id, err := c.CreateVLAN(ctx, "site1", cfg)
	if err != nil {
		t.Fatalf("CreateVLAN: %v", err)
	}

	vlan, err := c.FindVLAN(ctx, "site1", id)
	if err != nil {
		t.Fatalf("FindVLAN: %v", err)
	}
	if vlan == nil || vlan.Name != "IoT" || vlan.VLANID != 20 {
		t.Fatalf("FindVLAN = %+v, want IoT VLAN 20", vlan)
	}

	cfg.Name = "Internet of Things"
	if err := c.UpdateVLAN(ctx, "site1", vlan.ID, cfg); err != nil {
		t.Fatalf("UpdateVLAN: %v", err)
	}
	vlan, err = c.FindVLAN(ctx, "site1", id)
	if err != nil {
		t.Fatalf("FindVLAN after update: %v", err)
	}
	if vlan.Name != "Internet of Things" || vlan.VLANID != 20 {
		t.Fatalf("updated VLAN = %+v", vlan)
	}

	if err := c.DeleteVLAN(ctx, "site1", id); err != nil {
		t.Fatalf("DeleteVLAN: %v", err)
	}
	vlan, err = c.FindVLAN(ctx, "site1", id)
	if err != nil {
		t.Fatalf("FindVLAN after delete: %v", err)
	}
	if vlan != nil {
		t.Fatalf("VLAN still exists after delete: %+v", vlan)
	}
}

// purpose is numeric on the OpenAPI surface, so ListVLANs no longer filters on
// it: every LAN interface VLAN is returned, the seeded primary one included.
func TestListVLANsReturnsAllLANNetworks(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	m.vlans["interface-1"] = map[string]any{"id": "interface-1", "name": "LAN", "purpose": 1, "vlan": 1}
	m.vlans["vlan-20"] = map[string]any{"id": "vlan-20", "name": "IoT", "purpose": 1, "vlan": 20}
	srv := m.start()
	defer srv.Close()

	vlans, err := newTestClient(t, srv).ListVLANs(context.Background(), "site1")
	if err != nil {
		t.Fatalf("ListVLANs: %v", err)
	}
	if len(vlans) != 3 {
		t.Fatalf("ListVLANs = %+v, want all LAN interface VLANs", vlans)
	}
}

// A new VLAN must be bound to the gateway's LAN ports; the controller rejects
// an unbound LAN network, and an update must not silently rebind it.
func TestCreateVLANInheritsPrimaryLANInterfaces(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	ctx := context.Background()
	cfg := VLANConfig{Name: "IoT", DeviceMAC: testGatewayMAC, DeviceType: 1, VLANType: 0, VLANID: 20, GatewaySubnet: "192.0.2.1/24", DHCPEnabled: true, DHCPRangeStart: "192.0.2.100", DHCPRangeEnd: "192.0.2.199", DHCPDNSMode: "auto", DHCPLeaseTime: 1440}
	id, err := c.CreateVLAN(ctx, "site1", cfg)
	if err != nil {
		t.Fatalf("CreateVLAN: %v", err)
	}

	want := m.vlans[testPrimaryLANID]["interfaceIds"].([]any)
	m.mu.Lock()
	got := m.vlans[id]["interfaceIds"]
	m.mu.Unlock()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("created VLAN interfaceIds = %v, want %v", got, want)
	}

	cfg.Name = "Internet of Things"
	if err := c.UpdateVLAN(ctx, "site1", id, cfg); err != nil {
		t.Fatalf("UpdateVLAN: %v", err)
	}
	m.mu.Lock()
	got = m.vlans[id]["interfaceIds"]
	m.mu.Unlock()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("VLAN rebound by update: interfaceIds = %v, want %v", got, want)
	}
}

// Import relies on the read path hydrating every modelled attribute; if it
// returns only identity fields, an imported VLAN plans a replacement.
func TestFindVLANHydratesFullConfig(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	ctx := context.Background()
	cfg := VLANConfig{
		Name: "IoT", DeviceMAC: testGatewayMAC, DeviceType: 1, VLANType: 0, VLANID: 20,
		GatewaySubnet: "192.0.2.1/24", DHCPEnabled: true, DHCPRangeStart: "192.0.2.100",
		DHCPRangeEnd: "192.0.2.199", DHCPDNSMode: "manual", DHCPPrimaryDNS: "192.0.2.53",
		DHCPLeaseTime: 1440, Isolation: true,
	}
	id, err := c.CreateVLAN(ctx, "site1", cfg)
	if err != nil {
		t.Fatalf("CreateVLAN: %v", err)
	}
	found, err := c.FindVLAN(ctx, "site1", id)
	if err != nil {
		t.Fatalf("FindVLAN: %v", err)
	}
	got := VLANConfig{
		Name: found.Name, DeviceMAC: found.DeviceMAC, DeviceType: 1, VLANType: 0, VLANID: found.VLANID,
		GatewaySubnet: found.GatewaySubnet, DHCPEnabled: found.DHCPEnabled, DHCPRangeStart: found.DHCPRangeStart,
		DHCPRangeEnd: found.DHCPRangeEnd, DHCPDNSMode: found.DHCPDNSMode, DHCPPrimaryDNS: found.DHCPPrimaryDNS,
		DHCPLeaseTime: found.DHCPLeaseTime, Isolation: found.Isolation,
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("FindVLAN hydrated\n got %+v\nwant %+v", got, cfg)
	}
}
