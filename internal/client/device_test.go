package client

import (
	"context"
	"testing"
)

func TestListDevicesUsesBareArrayEndpoint(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)

	devices, err := c.ListDevices(context.Background(), "site1")
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 || devices[0].Type != "switch" || devices[0].Model != "SG2210P" {
		t.Fatalf("devices = %#v; want the seeded switch", devices)
	}
}
