package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newObjectUpdateTestClient(t *testing.T, endpoint string, current map[string]any, wantMethod string, captured *map[string]any) (*Client, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			writeEnvelope(w, 0, "", map[string]any{"omadacId": testOmadacID, "controllerVer": "6.3.0.45"})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
			writeEnvelope(w, 0, "", map[string]any{"token": "token"})
		case r.Method == http.MethodGet && r.URL.Path == endpoint:
			writeEnvelope(w, 0, "", map[string]any{"currentPage": 1, "currentSize": 1, "totalRows": 1, "data": []any{current}})
		case r.Method == wantMethod && r.URL.Path == endpoint+"/object-1":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding update body: %v", err)
			}
			*captured = body
			writeEnvelope(w, 0, "", nil)
		default:
			http.NotFound(w, r)
		}
	}))
	c, err := NewClient(server.URL, "admin", "password", false)
	if err != nil {
		server.Close()
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Authenticate(context.Background()); err != nil {
		server.Close()
		t.Fatalf("Authenticate: %v", err)
	}
	return c, server.Close
}

func TestUpdateSSIDPreservesPasswordAndUnknownFields(t *testing.T) {
	current := map[string]any{
		"id": "object-1", "name": "old", "band": 3, "security": 3,
		"broadcast": true, "vlanEnable": true, "vlanId": 10,
		"pskSetting":      map[string]any{"securityKey": "existing-secret", "unknownNested": true},
		"controllerOwned": map[string]any{"keep": "me"},
	}
	var captured map[string]any
	endpoint := fmt.Sprintf("/%s/api/v2/sites/site-1/setting/wlans/group-1/ssids", testOmadacID)
	c, closeServer := newObjectUpdateTestClient(t, endpoint, current, http.MethodPatch, &captured)
	defer closeServer()

	err := c.UpdateSSID(context.Background(), "site-1", "group-1", "object-1", SSIDConfig{
		Name: "new", Band: 7, Security: 3, Broadcast: false, VLANEnable: true, VLANID: 20,
	})
	if err != nil {
		t.Fatalf("UpdateSSID: %v", err)
	}
	psk, ok := captured["pskSetting"].(map[string]any)
	if !ok || psk["securityKey"] != "existing-secret" || psk["unknownNested"] != true {
		t.Fatalf("PSK settings were not preserved: %#v", captured["pskSetting"])
	}
	if captured["controllerOwned"] == nil {
		t.Fatal("controller-owned root field was dropped")
	}
	if captured["name"] != "new" || captured["vlanId"] != float64(20) {
		t.Fatalf("managed fields were not updated: %#v", captured)
	}
}

func TestUpdateACLDeepMergesDirectionAndPreservesUnknownFields(t *testing.T) {
	current := map[string]any{
		"id": "object-1", "type": 0, "name": "old", "status": true, "policy": 0,
		"protocols": []any{float64(256)}, "sourceType": 0, "sourceIds": []any{"source"},
		"destinationType": 0, "destinationIds": []any{"destination"},
		"direction":      map[string]any{"lanToWan": true, "lanToLan": false, "controllerDirectionFlag": "keep"},
		"customAclPorts": []any{map[string]any{"port": 443}}, "stateMode": float64(2),
	}
	var captured map[string]any
	endpoint := fmt.Sprintf("/%s/api/v2/sites/site-1/setting/firewall/acls", testOmadacID)
	c, closeServer := newObjectUpdateTestClient(t, endpoint, current, http.MethodPut, &captured)
	defer closeServer()

	err := c.UpdateACL(context.Background(), "site-1", "object-1", ACLConfig{
		Type: ACLTypeGateway, Name: "new", Enabled: false, Policy: 1, Protocols: []int64{6},
		SourceType: 0, SourceIDs: []string{"source"}, DestinationType: 0, DestinationIDs: []string{"destination"},
		Direction: ACLDirection{LANToWAN: false, LANToLAN: true},
	})
	if err != nil {
		t.Fatalf("UpdateACL: %v", err)
	}
	direction, ok := captured["direction"].(map[string]any)
	if !ok || direction["controllerDirectionFlag"] != "keep" || direction["lanToLAN"] != nil {
		t.Fatalf("direction was not safely deep-merged: %#v", captured["direction"])
	}
	if direction["lanToLan"] != true || direction["lanToWan"] != false {
		t.Fatalf("managed direction fields were not updated: %#v", direction)
	}
	if captured["customAclPorts"] == nil || captured["stateMode"] != float64(2) {
		t.Fatalf("controller-owned ACL fields were not preserved: %#v", captured)
	}
}
