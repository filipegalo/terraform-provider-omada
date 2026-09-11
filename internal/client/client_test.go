package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const testOmadacID = "omada1"

type envelopeResponse struct {
	ErrorCode int    `json:"errorCode"`
	Msg       string `json:"msg"`
	Result    any    `json:"result"`
}

func writeEnvelope(w http.ResponseWriter, errorCode int, msg string, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(envelopeResponse{ErrorCode: errorCode, Msg: msg, Result: result})
}

// mockServer is a stateful fake of the pieces of the Omada Controller API
// this client talks to: /api/info, login, and the classic sites list and
// DHCP reservation endpoints.
type mockServer struct {
	t             *testing.T
	controllerVer string

	mu             sync.Mutex
	loginCount     int
	expireNextAuth bool
	sites          []Site
	reservations   map[string]DHCPReservation // keyed by lowercase MAC
	vlans          map[string]map[string]any  // keyed by network ID
	ports          map[string]SwitchPort      // keyed by switch MAC and physical port
	profiles       map[string]map[string]any  // keyed by port-profile ID
	devices        []Device
	lastPutPath    string // last PUT request path, for asserting what key an update used
	lastPatchPath  string
}

const (
	testGatewayMAC   = "AA-BB-CC-DD-EE-FF"
	testPrimaryLANID = "lan-primary"
)

func newMockServer(t *testing.T, controllerVer string) *mockServer {
	t.Helper()
	return &mockServer{
		t:             t,
		controllerVer: controllerVer,
		reservations:  map[string]DHCPReservation{},
		// Every Omada site has a primary LAN network spanning the gateway's
		// LAN ports; it is where a new VLAN's interface binding comes from.
		vlans: map[string]map[string]any{
			testPrimaryLANID: {
				"id": testPrimaryLANID, "name": "Management(Default)", "purpose": 1, "vlan": 1,
				"primary": true, "deviceMac": testGatewayMAC,
				"interfaceIds": []any{"2_aaaa", "3_bbbb", "4_cccc", "5_dddd"},
			},
		},
		ports: map[string]SwitchPort{
			switchPortTestKey("D8-44-89-38-C6-C0", 2): {
				ID: "port-object-id", Port: 2, SwitchMAC: "D8-44-89-38-C6-C0", Name: "Camera",
				TagIDs: []string{"tag-security"}, NativeNetworkID: "network-security", NetworkTagsSetting: 1,
				ProfileID: "profile-security", ProfileOverrideEnable: false, ProfileVLANOverrideEnable: false,
				LinkSpeed: 0, Duplex: 0,
			},
		},
		profiles: map[string]map[string]any{},
		devices: []Device{{
			Name: "Main Switch", Type: "switch", Model: "SG2210P", MAC: "D8-44-89-38-C6-C0", IP: "10.20.1.2", StatusCategory: 1,
		}},
	}
}

func (m *mockServer) start() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(m.route))
}

func (m *mockServer) route(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/info":
		writeEnvelope(w, 0, "", map[string]any{
			"omadacId":      testOmadacID,
			"controllerVer": m.controllerVer,
		})
	case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
		m.mu.Lock()
		m.loginCount++
		token := fmt.Sprintf("token-%d", m.loginCount)
		m.mu.Unlock()
		writeEnvelope(w, 0, "", map[string]any{"token": token})
	case r.URL.Path == fmt.Sprintf("/%s/api/v2/sites", testOmadacID):
		if !m.checkAuth(w, r) {
			return
		}
		m.serveSites(w, r)
	case strings.Contains(r.URL.Path, "/openapi/") && strings.HasSuffix(r.URL.Path, "/networks/param-check"):
		if !m.checkAuth(w, r) {
			return
		}
		m.serveNetworkParamCheck(w, r)
	case strings.Contains(r.URL.Path, "/openapi/") && strings.Contains(r.URL.Path, "/switches/"):
		if !m.checkAuth(w, r) {
			return
		}
		m.serveSwitchPortPatch(w, r)
	case strings.Contains(r.URL.Path, "/openapi/") && strings.Contains(r.URL.Path, lanNetworksEndpoint):
		if !m.checkAuth(w, r) {
			return
		}
		m.serveVLANs(w, r)
	case strings.HasPrefix(r.URL.Path, fmt.Sprintf("/%s/api/v2/sites/", testOmadacID)):
		if !m.checkAuth(w, r) {
			return
		}
		if strings.Contains(r.URL.Path, lanNetworksEndpoint) {
			m.serveVLANs(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/devices") {
			m.mu.Lock()
			devices := append([]Device(nil), m.devices...)
			m.mu.Unlock()
			writeEnvelope(w, 0, "", devices)
			return
		}
		if strings.Contains(r.URL.Path, "/switches/") {
			m.serveSwitchPorts(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/setting/lan/profiles") {
			m.serveSwitchPortProfiles(w, r)
			return
		}
		m.serveDHCP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (m *mockServer) serveSwitchPortProfiles(w http.ResponseWriter, r *http.Request) {
	const endpoint = "/setting/lan/profiles"
	idx := strings.Index(r.URL.Path, endpoint)
	suffix := strings.Trim(strings.TrimPrefix(r.URL.Path[idx:], endpoint), "/")
	switch r.Method {
	case http.MethodGet:
		m.mu.Lock()
		all := make([]map[string]any, 0, len(m.profiles))
		for _, profile := range m.profiles {
			all = append(all, profile)
		}
		m.mu.Unlock()
		page, size := m.paginationParams(r)
		start, end := pageBounds(len(all), page, size)
		writeEnvelope(w, 0, "", map[string]any{"currentPage": page, "currentSize": size, "totalRows": len(all), "data": all[start:end]})
	case http.MethodPost:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			m.t.Fatalf("decoding port profile create body: %v", err)
		}
		id := fmt.Sprintf("profile-%d", len(m.profiles)+1)
		body["id"] = id
		body["prohibitModify"] = false
		body["poe"] = float64(2)
		body["spanningTreeSetting"] = map[string]any{"priority": float64(128), "instances": []any{map[string]any{"id": "mst-1"}}}
		m.mu.Lock()
		m.profiles[id] = body
		m.mu.Unlock()
		writeEnvelope(w, 0, "", nil)
	case http.MethodPatch:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			m.t.Fatalf("decoding port profile update body: %v", err)
		}
		m.mu.Lock()
		_, ok := m.profiles[suffix]
		if ok {
			m.profiles[suffix] = body
		}
		m.mu.Unlock()
		if !ok {
			writeEnvelope(w, -4, "profile not found", nil)
			return
		}
		writeEnvelope(w, 0, "", nil)
	case http.MethodDelete:
		m.mu.Lock()
		delete(m.profiles, suffix)
		m.mu.Unlock()
		writeEnvelope(w, 0, "", nil)
	default:
		http.NotFound(w, r)
	}
}

func switchPortTestKey(mac string, port int64) string {
	return fmt.Sprintf("%s:%d", NormalizeSwitchMAC(mac), port)
}

func (m *mockServer) serveSwitchPorts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/ports") {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/ports"), "/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	mac := parts[len(parts)-1]
	m.mu.Lock()
	ports := make([]SwitchPort, 0)
	for _, port := range m.ports {
		if strings.EqualFold(port.SwitchMAC, mac) {
			ports = append(ports, port)
		}
	}
	m.mu.Unlock()
	writeEnvelope(w, 0, "", ports)
}

func (m *mockServer) serveSwitchPortPatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Omada-Request-Source") != "web-local" {
		m.t.Error("switch port PATCH is missing Omada-Request-Source: web-local")
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		m.t.Fatalf("decoding switch port PATCH: %v", err)
	}
	allowed := map[string]bool{"name": true, "tagIds": true, "nativeNetworkId": true, "networkTagsSetting": true, "profileId": true, "profileOverrideEnable": true, "profileVlanOverrideEnable": true, "linkSpeed": true, "duplex": true}
	if len(body) != len(allowed) {
		writeEnvelope(w, -1001, "invalid switch port request body", nil)
		return
	}
	for field := range body {
		if !allowed[field] {
			writeEnvelope(w, -1001, "read-only switch port field: "+field, nil)
			return
		}
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	portNumber, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	mac := parts[len(parts)-3]
	key := switchPortTestKey(mac, portNumber)
	m.mu.Lock()
	existing, ok := m.ports[key]
	if ok {
		data, _ := json.Marshal(body)
		if err := json.Unmarshal(data, &existing); err != nil {
			m.t.Fatalf("decoding switch port PATCH fields: %v", err)
		}
		// The partial decode above leaves identity alone only because it starts
		// from the existing object, matching the controller's PATCH semantics.
		existing.ID, existing.Port, existing.SwitchMAC = m.ports[key].ID, m.ports[key].Port, m.ports[key].SwitchMAC
		m.ports[key] = existing
		m.lastPatchPath = r.URL.Path
	}
	m.mu.Unlock()
	if !ok {
		writeEnvelope(w, -39701, "This port does not exist", nil)
		return
	}
	writeEnvelope(w, 0, "", map[string]any{})
}

// hasLANInterfaces mirrors the controller's refusal to store a LAN network
// that is not bound to at least one gateway interface (API error -33515).
func hasLANInterfaces(body map[string]any) bool {
	ids, ok := body["interfaceIds"].([]any)
	return ok && len(ids) > 0
}

// serveNetworkParamCheck mirrors the controller's own body validation: it
// rejects a network body missing a discriminator the API treats as mandatory,
// so an incomplete request shape fails in tests rather than against the live
// controller.
func (m *mockServer) serveNetworkParamCheck(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		m.t.Fatalf("decoding param-check body: %v", err)
	}
	for _, field := range []string{"purpose", "name", "vlan", "deviceMac", "gatewaySubnet"} {
		if _, ok := body[field]; !ok {
			writeEnvelope(w, -1001, fmt.Sprintf("Parameter [%s] should not be empty", field), nil)
			return
		}
	}
	writeEnvelope(w, 0, "", map[string]any{})
}

func (m *mockServer) serveVLANs(w http.ResponseWriter, r *http.Request) {
	idx := strings.Index(r.URL.Path, lanNetworksEndpoint)
	suffix := strings.Trim(strings.TrimPrefix(r.URL.Path[idx:], lanNetworksEndpoint), "/")

	switch r.Method {
	case http.MethodGet:
		if suffix != "" {
			http.NotFound(w, r)
			return
		}
		m.mu.Lock()
		all := make([]map[string]any, 0, len(m.vlans))
		for _, vlan := range m.vlans {
			all = append(all, vlan)
		}
		m.mu.Unlock()
		page, size := 1, pageSize
		if r.URL.Query().Get("page") == "" {
			page, size = m.paginationParams(r)
		}
		start, end := pageBounds(len(all), page, size)
		writeEnvelope(w, 0, "", map[string]any{"currentPage": page, "currentSize": size, "totalRows": len(all), "data": all[start:end]})
	case http.MethodPost:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			m.t.Fatalf("decoding VLAN create body: %v", err)
		}
		if !hasLANInterfaces(body) {
			writeEnvelope(w, -33515, "LAN interfaces could not be none.", nil)
			return
		}
		id := fmt.Sprintf("vlan-%d", len(m.vlans)+1)
		body["id"] = id
		m.mu.Lock()
		m.vlans[id] = body
		m.mu.Unlock()
		writeEnvelope(w, 0, "", map[string]any{"id": id})
	case http.MethodPatch, http.MethodPut:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			m.t.Fatalf("decoding VLAN update body: %v", err)
		}
		if !hasLANInterfaces(body) {
			writeEnvelope(w, -33515, "LAN interfaces could not be none.", nil)
			return
		}
		m.mu.Lock()
		existing, ok := m.vlans[suffix]
		if ok {
			body["id"] = existing["id"]
			m.vlans[suffix] = body
		}
		m.mu.Unlock()
		if !ok {
			writeEnvelope(w, -4, "VLAN not found", nil)
			return
		}
		writeEnvelope(w, 0, "", map[string]any{})
	case http.MethodDelete:
		m.mu.Lock()
		delete(m.vlans, suffix)
		m.mu.Unlock()
		writeEnvelope(w, 0, "", map[string]any{})
	default:
		http.NotFound(w, r)
	}
}

// checkAuth validates the token query param and Csrf-Token header the
// client is required to send on every authenticated request, and consumes
// a pending simulated session expiry if one was armed.
func (m *mockServer) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	m.t.Helper()

	token := r.URL.Query().Get("token")
	csrf := r.Header.Get("Csrf-Token")
	if token == "" || csrf == "" {
		m.t.Errorf("request %s missing token/Csrf-Token: token=%q csrf=%q", r.URL.Path, token, csrf)
		writeEnvelope(w, -2, "missing auth", nil)
		return false
	}
	if token != csrf {
		m.t.Errorf("token %q and Csrf-Token %q must match", token, csrf)
	}

	m.mu.Lock()
	expire := m.expireNextAuth
	if expire {
		m.expireNextAuth = false
	}
	m.mu.Unlock()

	if expire {
		writeEnvelope(w, -1, "session expired", nil)
		return false
	}
	return true
}

// paginationParams asserts the client always sends the classic API's
// currentPage/currentPageSize parameters -- confirmed live against a real
// 6.3.0.45 controller, which silently mishandles page/pageSize instead of
// rejecting it outright (see pagination.go).
func (m *mockServer) paginationParams(r *http.Request) (page, size int) {
	m.t.Helper()
	q := r.URL.Query()

	if q.Get("page") != "" || q.Get("pageSize") != "" {
		m.t.Errorf("controller %s should receive currentPage/currentPageSize, got page/pageSize", m.controllerVer)
	}
	page, _ = strconv.Atoi(q.Get("currentPage"))
	size, _ = strconv.Atoi(q.Get("currentPageSize"))
	if size == 0 {
		size = pageSize
	}
	if page == 0 {
		page = 1
	}
	return
}

func (m *mockServer) serveSites(w http.ResponseWriter, r *http.Request) {
	page, size := m.paginationParams(r)
	start, end := pageBounds(len(m.sites), page, size)

	writeEnvelope(w, 0, "", map[string]any{
		"currentPage": page,
		"currentSize": size,
		"totalRows":   len(m.sites),
		"data":        m.sites[start:end],
	})
}

func (m *mockServer) serveDHCP(w http.ResponseWriter, r *http.Request) {
	idx := strings.Index(r.URL.Path, dhcpReservationEndpoint)
	suffix := strings.Trim(strings.TrimPrefix(r.URL.Path[idx:], dhcpReservationEndpoint), "/")

	switch r.Method {
	case http.MethodGet:
		if suffix != "" {
			http.NotFound(w, r)
			return
		}
		m.mu.Lock()
		all := make([]DHCPReservation, 0, len(m.reservations))
		for _, res := range m.reservations {
			all = append(all, res)
		}
		m.mu.Unlock()

		page, size := m.paginationParams(r)
		start, end := pageBounds(len(all), page, size)
		writeEnvelope(w, 0, "", map[string]any{
			"currentPage": page,
			"currentSize": size,
			"totalRows":   len(all),
			"data":        all[start:end],
		})
	case http.MethodPost:
		var req CreateDHCPReservationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			m.t.Fatalf("decoding create body: %v", err)
		}
		m.mu.Lock()
		m.reservations[normalizeMAC(req.MAC)] = DHCPReservation{
			MAC: req.MAC, IP: req.IP, NetID: req.NetID, Status: req.Status, Description: req.Description, Name: req.Name,
		}
		m.mu.Unlock()
		writeEnvelope(w, 0, "", map[string]any{})
	case http.MethodPut:
		// Confirmed live: the classic endpoint's update is a full-object
		// PUT keyed by MAC in the path, not a partial PATCH -- so the mock
		// decodes and stores a whole DHCPReservation, replacing whatever
		// was there, exactly like the real controller does. Keyed via
		// normalizeMAC, like every other lookup here: a reservation's
		// stored MAC field and this map's key aren't necessarily the same
		// notation (tests deliberately exercise dash- and colon-separated
		// MACs interchangeably, the way a real controller's dash-separated
		// data and this provider's colon-separated input do).
		mac := normalizeMAC(suffix)
		var res DHCPReservation
		if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
			m.t.Fatalf("decoding update body: %v", err)
		}
		m.mu.Lock()
		m.lastPutPath = r.URL.Path
		_, ok := m.reservations[mac]
		if ok {
			m.reservations[mac] = res
		}
		m.mu.Unlock()
		if !ok {
			writeEnvelope(w, -4, "reservation not found", nil)
			return
		}
		writeEnvelope(w, 0, "", map[string]any{})
	case http.MethodDelete:
		m.mu.Lock()
		delete(m.reservations, normalizeMAC(suffix))
		m.mu.Unlock()
		writeEnvelope(w, 0, "", map[string]any{})
	default:
		http.NotFound(w, r)
	}
}

func pageBounds(total, page, size int) (start, end int) {
	start = (page - 1) * size
	if start > total {
		start = total
	}
	end = start + size
	if end > total {
		end = total
	}
	return
}

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := NewClient(srv.URL, "admin", "hunter2", "", "", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Authenticate(context.Background()); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	return c
}

func TestAuthenticate(t *testing.T) {
	m := newMockServer(t, "5.15.20.6")
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)

	if c.omadacID != testOmadacID {
		t.Errorf("omadacID = %q, want %q", c.omadacID, testOmadacID)
	}
	if c.controllerVer != "5.15.20.6" {
		t.Errorf("controllerVer = %q, want %q", c.controllerVer, "5.15.20.6")
	}
	if c.token == "" {
		t.Error("token was not populated")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loginCount != 1 {
		t.Errorf("loginCount = %d, want 1", m.loginCount)
	}
}

func TestSessionExpiryRetriesOnce(t *testing.T) {
	m := newMockServer(t, "6.2.0.0")
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)

	m.mu.Lock()
	m.reservations["aa:bb:cc:dd:ee:ff"] = DHCPReservation{MAC: "AA:BB:CC:DD:EE:FF", IP: "10.0.0.5"}
	m.expireNextAuth = true
	m.mu.Unlock()

	reservations, err := c.ListDHCPReservations(context.Background(), "site1")
	if err != nil {
		t.Fatalf("ListDHCPReservations: %v", err)
	}
	if len(reservations) != 1 {
		t.Fatalf("got %d reservations, want 1", len(reservations))
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loginCount != 2 {
		t.Errorf("loginCount = %d, want 2 (initial + retry)", m.loginCount)
	}
}

func TestResolveSite(t *testing.T) {
	m := newMockServer(t, "5.15.20.6")
	m.sites = []Site{{ID: "site-1", Name: "Home"}, {ID: "site-2", Name: "Office"}}
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	ctx := context.Background()

	id, err := c.ResolveSite(ctx, "Office")
	if err != nil {
		t.Fatalf("ResolveSite by name: %v", err)
	}
	if id != "site-2" {
		t.Errorf("ResolveSite by name = %q, want site-2", id)
	}

	id, err = c.ResolveSite(ctx, "site-1")
	if err != nil {
		t.Fatalf("ResolveSite by ID: %v", err)
	}
	if id != "site-1" {
		t.Errorf("ResolveSite by ID = %q, want site-1", id)
	}

	if _, err := c.ResolveSite(ctx, "nope"); err == nil {
		t.Error("expected error resolving unknown site")
	}
}

func TestDHCPReservationCRUD(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	ctx := context.Background()
	const siteID = "site1"

	if err := c.CreateDHCPReservation(ctx, siteID, CreateDHCPReservationRequest{
		MAC: "AA:BB:CC:DD:EE:FF", IP: "10.0.0.5", NetID: "net1", Status: true, Description: "printer",
	}); err != nil {
		t.Fatalf("CreateDHCPReservation: %v", err)
	}

	got, err := c.FindDHCPReservationByMAC(ctx, siteID, "aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("FindDHCPReservationByMAC: %v", err)
	}
	if got == nil || got.IP != "10.0.0.5" || got.Description != "printer" {
		t.Fatalf("FindDHCPReservationByMAC = %+v", got)
	}

	newIP := "10.0.0.6"
	if err := c.UpdateDHCPReservation(ctx, siteID, "AA:BB:CC:DD:EE:FF", UpdateDHCPReservationRequest{IP: &newIP}); err != nil {
		t.Fatalf("UpdateDHCPReservation: %v", err)
	}
	got, err = c.FindDHCPReservationByMAC(ctx, siteID, "aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("FindDHCPReservationByMAC after update: %v", err)
	}
	if got.IP != "10.0.0.6" || got.Description != "printer" {
		t.Fatalf("after update = %+v, want IP updated and description unchanged", got)
	}

	if err := c.DeleteDHCPReservation(ctx, siteID, "aa:bb:cc:dd:ee:ff"); err != nil {
		t.Fatalf("DeleteDHCPReservation: %v", err)
	}
	got, err = c.FindDHCPReservationByMAC(ctx, siteID, "aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("FindDHCPReservationByMAC after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("reservation still present after delete: %+v", got)
	}
}

// TestFindDHCPReservationByMACDashSeparated guards against a real bug found
// against a live controller: the classic endpoint returns MACs
// dash-separated ("AA-BB-CC-DD-EE-FF") regardless of how they were created,
// while every other input to this provider uses colons. A literal
// strings.EqualFold comparison between the two never matches.
func TestFindDHCPReservationByMACDashSeparated(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	m.reservations["aa:bb:cc:dd:ee:ff"] = DHCPReservation{MAC: "AA-BB-CC-DD-EE-FF", IP: "192.0.2.53"}
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.FindDHCPReservationByMAC(context.Background(), "site1", "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatalf("FindDHCPReservationByMAC: %v", err)
	}
	if got == nil {
		t.Fatal("FindDHCPReservationByMAC = nil, want a match despite the separator mismatch")
	}
	if got.IP != "192.0.2.53" {
		t.Errorf("IP = %q, want 192.0.2.53", got.IP)
	}
}

// TestUpdateDHCPReservationFullObjectPUT guards against a real bug found
// against a live controller: updating a reservation is a full-object PUT
// keyed by the reservation's own MAC in the URL (confirmed from the real
// Omada web UI's own edit request), not a partial PATCH and not keyed by
// the reservation's internal id (both were tried against a real controller
// and rejected). This checks the update targets the reservation's exact,
// dash-separated MAC -- not the caller's colon-separated input -- and that
// fields the caller didn't touch survive the full-object replace.
func TestUpdateDHCPReservationFullObjectPUT(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	m.reservations[normalizeMAC("aa:bb:cc:dd:ee:ff")] = DHCPReservation{
		MAC:                "AA-BB-CC-DD-EE-FF",
		IP:                 "10.0.0.5",
		NetID:              "net1",
		Status:             true,
		Description:        "printer",
		Name:               "printer",
		ServerMAC:          "11-22-33-44-55-66",
		ServerType:         "gateway",
		Options:            json.RawMessage(`[]`),
		FeatureDescription: json.RawMessage(`[{"feature":"dhcpReservation_options","featureState":2,"changeable":true}]`),
	}
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	newDescription := "front desk printer"
	if err := c.UpdateDHCPReservation(context.Background(), "site1", "aa:bb:cc:dd:ee:ff", UpdateDHCPReservationRequest{
		Description: &newDescription,
	}); err != nil {
		t.Fatalf("UpdateDHCPReservation: %v", err)
	}

	m.mu.Lock()
	putPath := m.lastPutPath
	m.mu.Unlock()
	if !strings.Contains(putPath, "AA-BB-CC-DD-EE-FF") {
		t.Errorf("PUT path = %q, want it keyed on the reservation's own dash-separated MAC", putPath)
	}
	if strings.Contains(putPath, "aa:bb:cc:dd:ee:ff") {
		t.Errorf("PUT path = %q, must not use the caller's colon-separated MAC", putPath)
	}

	got, err := c.FindDHCPReservationByMAC(context.Background(), "site1", "aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("FindDHCPReservationByMAC: %v", err)
	}
	if got.Description != "front desk printer" {
		t.Errorf("Description = %q, want %q", got.Description, "front desk printer")
	}
	if got.ServerMAC != "11-22-33-44-55-66" || got.ServerType != "gateway" {
		t.Errorf("update clobbered a field it wasn't asked to change: %+v", got)
	}
	if len(got.FeatureDescription) == 0 {
		t.Error("FeatureDescription was dropped by the update")
	}
}

// TestDHCPReservationNameMirrorsDescription guards against a real problem
// found live: left to the controller, a reservation's auto-generated `name`
// ends up an oddly cased, oddly punctuated string unrelated to what was
// asked for (e.g. "Shelly---Kitchen-Sockets" for a description of
// "shelly-kitchen-sockets"). Create always sets name = description, and
// Update re-syncs it on every call -- even one that doesn't touch
// Description -- so a reservation created before this fix self-heals the
// next time anything about it changes.
func TestDHCPReservationNameMirrorsDescription(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	ctx := context.Background()
	const siteID = "site1"

	if err := c.CreateDHCPReservation(ctx, siteID, CreateDHCPReservationRequest{
		MAC: "AA:BB:CC:DD:EE:01", IP: "10.0.0.9", NetID: "net1", Status: true, Description: "shelly-kitchen-sockets",
	}); err != nil {
		t.Fatalf("CreateDHCPReservation: %v", err)
	}
	got, err := c.FindDHCPReservationByMAC(ctx, siteID, "aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("FindDHCPReservationByMAC: %v", err)
	}
	if got.Name != "shelly-kitchen-sockets" {
		t.Errorf("after create, Name = %q, want it to mirror Description", got.Name)
	}

	// Simulate a reservation the controller auto-named before this fix, then
	// confirm any update self-heals it even without touching Description.
	m.mu.Lock()
	rec := m.reservations[normalizeMAC("aa:bb:cc:dd:ee:01")]
	rec.Name = "Shelly---Kitchen-Sockets"
	m.reservations[normalizeMAC("aa:bb:cc:dd:ee:01")] = rec
	m.mu.Unlock()

	newIP := "10.0.0.10"
	if err := c.UpdateDHCPReservation(ctx, siteID, "aa:bb:cc:dd:ee:01", UpdateDHCPReservationRequest{IP: &newIP}); err != nil {
		t.Fatalf("UpdateDHCPReservation: %v", err)
	}
	got, err = c.FindDHCPReservationByMAC(ctx, siteID, "aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("FindDHCPReservationByMAC after update: %v", err)
	}
	if got.Name != "shelly-kitchen-sockets" {
		t.Errorf("after update, Name = %q, want it self-healed to match Description", got.Name)
	}
}

func TestDHCPReservationListPagination(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	for i := 0; i < 150; i++ {
		mac := fmt.Sprintf("aa:bb:cc:dd:ee:%02x", i)
		m.reservations[mac] = DHCPReservation{MAC: mac, IP: fmt.Sprintf("10.0.0.%d", i)}
	}
	srv := m.start()
	defer srv.Close()

	c := newTestClient(t, srv)
	reservations, err := c.ListDHCPReservations(context.Background(), "site1")
	if err != nil {
		t.Fatalf("ListDHCPReservations: %v", err)
	}
	if len(reservations) != 150 {
		t.Fatalf("got %d reservations, want 150 (pagination should have followed 2 pages)", len(reservations))
	}
}
