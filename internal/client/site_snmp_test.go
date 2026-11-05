package client

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// serveSiteSNMP mirrors the SNMP service setting of a 6.3.0.111 controller:
// GET returns the credentials next to the switches, and PATCH rejects a body
// without both switches but keeps every field it leaves out.
func (m *mockServer) serveSiteSNMP(w http.ResponseWriter, r *http.Request) {
	siteID := strings.Split(strings.SplitN(r.URL.Path, "/sites/", 2)[1], "/")[0]
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.siteSNMP[siteID]
	if !ok {
		writeEnvelope(w, -1505, "The current user does not have permissions to access this site.", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeEnvelope(w, 0, "", current)
	case http.MethodPatch:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			m.t.Fatalf("decoding SNMP PATCH: %v", err)
		}
		m.lastPatchPath, m.lastPatchBody = r.URL.Path, body
		for _, field := range []string{"snmpV1V2CEnable", "snmpV3Enable"} {
			if body[field] == nil {
				writeEnvelope(w, -1001, "Parameter ["+field+"] should not be null.", nil)
				return
			}
		}
		for field, value := range body {
			current[field] = value
		}
		writeEnvelope(w, 0, "", nil)
	default:
		http.NotFound(w, r)
	}
}

func testSiteSNMPDocument() map[string]any {
	return map[string]any{
		"snmpV1V2CEnable": false, "communityString": "example-community",
		"snmpV3Enable": true, "username": "example-user", "password": "example-password",
		"securityLevel": float64(2), "authMode": float64(0), "privacyMode": float64(0), "privacyPassword": "example-privacy",
		"location": "", "contact": "",
	}
}

// TestUpdateSiteSNMPSendsOnlyTheSwitches guards the two promises of the SNMP
// client: both switches are always sent, and no credential ever is, so the
// controller keeps the ones it has.
func TestUpdateSiteSNMPSendsOnlyTheSwitches(t *testing.T) {
	m := newMockServer(t, "6.3.0.111")
	m.siteSNMP[testSiteID] = testSiteSNMPDocument()
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)
	ctx := context.Background()

	got, err := c.GetSiteSNMP(ctx, testSiteID)
	if err != nil || *got != (SiteSNMP{V1V2CEnable: false, V3Enable: true}) {
		t.Fatalf("GetSiteSNMP = %+v, %v; want v1/v2c off, v3 on", got, err)
	}
	if err := c.UpdateSiteSNMP(ctx, testSiteID, SiteSNMP{V1V2CEnable: false, V3Enable: false}); err != nil {
		t.Fatalf("UpdateSiteSNMP: %v", err)
	}

	m.mu.Lock()
	body, stored := m.lastPatchBody, m.siteSNMP[testSiteID]
	m.mu.Unlock()
	if want := map[string]any{"snmpV1V2CEnable": false, "snmpV3Enable": false}; !reflect.DeepEqual(body, want) {
		t.Errorf("PATCH body = %#v, want only the two switches %#v", body, want)
	}
	want := testSiteSNMPDocument()
	want["snmpV3Enable"] = false
	if !reflect.DeepEqual(stored, want) {
		t.Errorf("SNMP setting after update = %#v; want only snmpV3Enable changed: %#v", stored, want)
	}
}
