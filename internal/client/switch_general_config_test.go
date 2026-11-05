package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// mockSwitch is the general configuration of one adopted switch. The two
// switch series are served from different paths and expose different fields.
type mockSwitch struct {
	easyManaged bool
	config      map[string]any
}

const (
	testSwitchMAC            = "AA-BB-CC-DD-EE-01"
	testEasyManagedSwitchMAC = "AA-BB-CC-DD-EE-02"
)

func decodeTestJSON(t *testing.T, document string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatalf("decoding test fixture: %v", err)
	}
	return decoded
}

// newTestSwitch returns a regular switch with every field the general-config
// endpoint returns for that series, as read from a 6.3.0.45 controller.
func newTestSwitch(t *testing.T) *mockSwitch {
	t.Helper()
	return &mockSwitch{config: decodeTestJSON(t, `{
		"name": "AA-BB-CC-DD-EE-01", "ledSetting": 2, "tagIds": [],
		"jumbo": 1518, "jumboFollowSite": true, "lagHashAlg": 2, "lagHashAlgFollowSite": true,
		"location": {}, "snmp": {"contact": "", "location": "", "type": 0}
	}`)}
}

// newTestEasyManagedSwitch returns an Agile Series switch with the reduced
// field set of the "es" general-config endpoint.
func newTestEasyManagedSwitch(t *testing.T) *mockSwitch {
	t.Helper()
	return &mockSwitch{easyManaged: true, config: decodeTestJSON(t, `{
		"name": "AA-BB-CC-DD-EE-02", "ledSetting": 2, "tagIds": [],
		"jumboEnable": false, "remember": true, "supportPowerAlert": false
	}`)}
}

var (
	switchGeneralConfigFields            = []string{"name", "ledSetting", "tagIds", "location", "jumboFollowSite", "jumbo", "lagHashAlgFollowSite", "lagHashAlg", "snmp", "sdm"}
	easyManagedSwitchGeneralConfigFields = []string{"name", "ledSetting", "tagIds", "jumboEnable", "location", "remember", "powerAlertEnable"}
)

// serveSwitchGeneralConfig mirrors the Open API's two general-config
// endpoints: .../switches/{mac}/general-config for regular switches and
// .../switches/es/{mac}/general-config for the Agile Series. PATCH is partial,
// exactly like the controller: only the fields in the body change.
func (m *mockServer) serveSwitchGeneralConfig(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/general-config"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	mac := parts[len(parts)-1]
	viaEasyManagedPath := parts[len(parts)-2] == "es"

	m.mu.Lock()
	defer m.mu.Unlock()
	device, ok := m.switches[NormalizeSwitchMAC(mac)]
	if !ok {
		writeEnvelope(w, errSwitchNotFound, "Switch does not exist", nil)
		return
	}
	if device.easyManaged && !viaEasyManagedPath {
		writeEnvelope(w, errAgileSwitchPath, "The Agile Series Switch should use the corresponding path.", nil)
		return
	}
	if !device.easyManaged && viaEasyManagedPath {
		// The controller does answer here, with the Easy Managed view of a
		// regular switch. The client must never rely on that.
		m.t.Errorf("regular switch %s was addressed through the Agile Series path", mac)
		writeEnvelope(w, -1001, "wrong switch series", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeEnvelope(w, 0, "", device.config)
	case http.MethodPatch:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			m.t.Fatalf("decoding general-config PATCH: %v", err)
		}
		allowed, maxNameLength := switchGeneralConfigFields, SwitchNameMaxLength
		if device.easyManaged {
			allowed, maxNameLength = easyManagedSwitchGeneralConfigFields, EasyManagedSwitchNameMaxLength
		}
		for field := range body {
			if !containsString(allowed, field) {
				writeEnvelope(w, -1001, "unknown general-config field: "+field, nil)
				return
			}
		}
		if name, ok := body["name"].(string); ok {
			if length := utf8.RuneCountInString(name); length < 1 || length > maxNameLength {
				writeEnvelope(w, -1001, fmt.Sprintf("Device name should contain 1 to %d characters.", maxNameLength), nil)
				return
			}
		}
		for field, value := range body {
			device.config[field] = value
		}
		m.lastPatchPath, m.lastPatchBody = r.URL.Path, body
		writeEnvelope(w, 0, "", map[string]any{})
	default:
		http.NotFound(w, r)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// TestSwitchGeneralConfigRenameSendsOnlyName guards the contract the resource
// relies on: a rename is a partial PATCH carrying the name and nothing else,
// so LED, jumbo, LAG, SNMP and location settings can never be reset by it.
func TestSwitchGeneralConfigRenameSendsOnlyName(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	m.switches[testSwitchMAC] = newTestSwitch(t)
	want := newTestSwitch(t).config
	want["name"] = "core-switch"
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)
	ctx := context.Background()

	// Colon-separated lowercase input must reach the controller's dash form.
	found, err := c.GetSwitchGeneralConfig(ctx, "site1", "aa:bb:cc:dd:ee:01")
	if err != nil || found == nil {
		t.Fatalf("GetSwitchGeneralConfig = %+v, %v; want the seeded switch", found, err)
	}
	if found.Name != testSwitchMAC || found.EasyManaged {
		t.Fatalf("GetSwitchGeneralConfig = %+v; want the unnamed regular switch", found)
	}

	name := "core-switch"
	if err := c.UpdateSwitchGeneralConfig(ctx, "site1", "aa:bb:cc:dd:ee:01", found.EasyManaged, SwitchGeneralConfigUpdate{Name: &name}); err != nil {
		t.Fatalf("UpdateSwitchGeneralConfig: %v", err)
	}

	m.mu.Lock()
	gotPath, gotBody, got := m.lastPatchPath, m.lastPatchBody, m.switches[testSwitchMAC].config
	m.mu.Unlock()
	wantPath := "/openapi/v1/omada1/sites/site1/switches/AA-BB-CC-DD-EE-01/general-config"
	if gotPath != wantPath {
		t.Errorf("PATCH path = %q, want %q", gotPath, wantPath)
	}
	if !reflect.DeepEqual(gotBody, map[string]any{"name": "core-switch"}) {
		t.Errorf("PATCH body = %#v; want only the name", gotBody)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("general config after rename = %#v; want only the name changed: %#v", got, want)
	}
}

// TestSwitchGeneralConfigEasyManagedFallsBackToESPath guards against the
// controller's error -39742: an Agile Series switch is not served from the
// regular path, and reads and writes have to move to the "es" one.
func TestSwitchGeneralConfigEasyManagedFallsBackToESPath(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	m.switches[testEasyManagedSwitchMAC] = newTestEasyManagedSwitch(t)
	want := newTestEasyManagedSwitch(t).config
	want["name"] = "desk-switch"
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)
	ctx := context.Background()

	found, err := c.GetSwitchGeneralConfig(ctx, "site1", testEasyManagedSwitchMAC)
	if err != nil || found == nil {
		t.Fatalf("GetSwitchGeneralConfig = %+v, %v; want the seeded Agile Series switch", found, err)
	}
	if found.Name != testEasyManagedSwitchMAC || !found.EasyManaged {
		t.Fatalf("GetSwitchGeneralConfig = %+v; want the unnamed Agile Series switch", found)
	}

	name := "desk-switch"
	if err := c.UpdateSwitchGeneralConfig(ctx, "site1", testEasyManagedSwitchMAC, found.EasyManaged, SwitchGeneralConfigUpdate{Name: &name}); err != nil {
		t.Fatalf("UpdateSwitchGeneralConfig: %v", err)
	}

	m.mu.Lock()
	gotPath, gotBody, got := m.lastPatchPath, m.lastPatchBody, m.switches[testEasyManagedSwitchMAC].config
	m.mu.Unlock()
	wantPath := "/openapi/v1/omada1/sites/site1/switches/es/AA-BB-CC-DD-EE-02/general-config"
	if gotPath != wantPath {
		t.Errorf("PATCH path = %q, want %q", gotPath, wantPath)
	}
	if !reflect.DeepEqual(gotBody, map[string]any{"name": "desk-switch"}) {
		t.Errorf("PATCH body = %#v; want only the name", gotBody)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("general config after rename = %#v; want only the name changed: %#v", got, want)
	}
}

func TestGetSwitchGeneralConfigUnknownSwitch(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)

	found, err := c.GetSwitchGeneralConfig(context.Background(), "site1", "AA-BB-CC-DD-EE-FF")
	if err != nil || found != nil {
		t.Fatalf("GetSwitchGeneralConfig = %+v, %v; want nil, nil for a MAC that is not an adopted switch", found, err)
	}
}

// TestUpdateSwitchGeneralConfigReportsControllerError checks that a rejected
// write surfaces the controller's code and message, and stays inspectable.
func TestUpdateSwitchGeneralConfigReportsControllerError(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	m.switches[testEasyManagedSwitchMAC] = newTestEasyManagedSwitch(t)
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)

	name := strings.Repeat("n", EasyManagedSwitchNameMaxLength+1)
	err := c.UpdateSwitchGeneralConfig(context.Background(), "site1", testEasyManagedSwitchMAC, true, SwitchGeneralConfigUpdate{Name: &name})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != -1001 {
		t.Fatalf("UpdateSwitchGeneralConfig error = %v; want API error -1001", err)
	}
	if want := "API error -1001: Device name should contain 1 to 32 characters."; err.Error() != want {
		t.Errorf("error text = %q, want %q", err.Error(), want)
	}
}

// TestSwitchGeneralConfigUsesOpenAPIAccessToken checks that an Open API
// application's access token is used when client credentials are configured,
// without the classic session token and without the web UI marker, and that
// the Agile Series fallback works the same way under that authentication.
func TestSwitchGeneralConfigUsesOpenAPIAccessToken(t *testing.T) {
	var patchedPath string
	var patchedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		regularPath := fmt.Sprintf("/openapi/v1/%s/sites/site-1/switches/%s/general-config", testOmadacID, testEasyManagedSwitchMAC)
		easyManagedPath := fmt.Sprintf("/openapi/v1/%s/sites/site-1/switches/es/%s/general-config", testOmadacID, testEasyManagedSwitchMAC)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			writeEnvelope(w, 0, "", map[string]any{"omadacId": testOmadacID, "controllerVer": "6.3.0.45"})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
			writeEnvelope(w, 0, "", map[string]any{"token": "classic-token"})
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/authorize/token":
			writeEnvelope(w, 0, "", map[string]any{"accessToken": "oauth-token", "expiresIn": 7200})
		case r.URL.Path == regularPath || r.URL.Path == easyManagedPath:
			if got := r.Header.Get("Authorization"); got != "AccessToken=oauth-token" {
				t.Errorf("Authorization = %q, want AccessToken=oauth-token", got)
			}
			if r.Header.Get("Csrf-Token") != "" || r.URL.Query().Get("token") != "" {
				t.Error("Open API request unexpectedly carried the classic session token")
			}
			if got := r.Header.Get("Omada-Request-Source"); got != "" {
				t.Errorf("Omada-Request-Source = %q, want empty with Open API auth", got)
			}
			switch {
			case r.URL.Path == regularPath:
				writeEnvelope(w, errAgileSwitchPath, "The Agile Series Switch should use the corresponding path.", nil)
			case r.Method == http.MethodGet:
				writeEnvelope(w, 0, "", map[string]any{"name": "desk-switch", "ledSetting": 2})
			case r.Method == http.MethodPatch:
				patchedPath = r.URL.Path
				if err := json.NewDecoder(r.Body).Decode(&patchedBody); err != nil {
					t.Errorf("decoding general-config PATCH: %v", err)
				}
				writeEnvelope(w, 0, "", map[string]any{})
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c, err := NewClient(server.URL, "admin", "password", "client-id", "client-secret", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()
	if err := c.Authenticate(ctx); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	found, err := c.GetSwitchGeneralConfig(ctx, "site-1", testEasyManagedSwitchMAC)
	if err != nil || found == nil || found.Name != "desk-switch" || !found.EasyManaged {
		t.Fatalf("GetSwitchGeneralConfig = %+v, %v; want the Agile Series switch", found, err)
	}
	name := "desk-switch-2"
	if err := c.UpdateSwitchGeneralConfig(ctx, "site-1", testEasyManagedSwitchMAC, found.EasyManaged, SwitchGeneralConfigUpdate{Name: &name}); err != nil {
		t.Fatalf("UpdateSwitchGeneralConfig: %v", err)
	}
	if !strings.Contains(patchedPath, "/switches/es/") {
		t.Errorf("PATCH path = %q, want the Agile Series path", patchedPath)
	}
	if !reflect.DeepEqual(patchedBody, map[string]any{"name": "desk-switch-2"}) {
		t.Errorf("PATCH body = %#v; want only the name", patchedBody)
	}
}
