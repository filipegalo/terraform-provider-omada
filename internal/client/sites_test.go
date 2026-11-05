package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

const testSiteID = "site-1"

// testSiteDocument is a site as the Open API returns it from a 6.3.0.45
// controller with DST switched off: computed DST timestamps and empty start
// and end objects next to the configuration, and no key at all for a setting
// that is unset (here the site tags).
const testSiteDocument = `{
	"siteId": "site-1", "name": "Default", "type": 0,
	"region": "United States", "timeZone": "America/New_York", "scenario": "Office",
	"ntpEnable": true, "ntpServers": ["pool.ntp.org"],
	"address": "1 Example Street", "longitude": -73.9857, "latitude": 40.7484,
	"supportES": true, "supportL2": true,
	"dst": {"enable": false, "mode": 0, "start": {}, "end": {}, "status": false,
		"startTime": 0, "endTime": 0, "nextStart": 0, "nextEnd": 0, "lastStart": 0, "lastEnd": 0}
}`

func isSiteDetailPath(path string) bool {
	id, ok := strings.CutPrefix(path, fmt.Sprintf("/openapi/v1/%s/sites/", testOmadacID))
	return ok && id != "" && !strings.Contains(id, "/")
}

var siteDSTTimeFields = []string{"month", "serial", "day", "hour", "minute"}

// serveSiteDetail mirrors the Open API's site endpoint. Like the controller,
// PUT takes the UpdateSiteEntity shape, which differs from what GET returns,
// and replaces the whole site: a field left out of the body is gone afterwards.
func (m *mockServer) serveSiteDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.siteDetails[id]
	if !ok {
		// The controller does not distinguish a missing site from one the
		// credentials may not access.
		writeEnvelope(w, -1505, "The current user does not have permissions to access this site.", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeEnvelope(w, 0, "", current)
	case http.MethodPut:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			m.t.Fatalf("decoding site PUT: %v", err)
		}
		m.lastPutPath, m.lastPutBody = r.URL.Path, body
		for _, field := range []string{"region", "scenario", "timeZone"} {
			if text, _ := body[field].(string); text == "" {
				writeEnvelope(w, -1001, fmt.Sprintf("Parameter [%s] should not be empty", field), nil)
				return
			}
		}
		// The update schema has neither field; a client that echoes the
		// document it read without removing them must fail here.
		for _, field := range []string{"siteId", "type"} {
			if _, ok := body[field]; ok {
				writeEnvelope(w, -1001, "read-only site field: "+field, nil)
				return
			}
		}
		name, _ := body["name"].(string)
		if length := utf8.RuneCountInString(name); length < 1 || length > 64 {
			writeEnvelope(w, -1001, "Site name should contain 1 to 64 characters.", nil)
			return
		}
		dst, message := mockSiteDST(body["dst"])
		if message != "" {
			writeEnvelope(w, -1001, message, nil)
			return
		}
		stored := map[string]any{"siteId": current["siteId"], "type": current["type"]}
		for field, value := range body {
			stored[field] = value
		}
		stored["dst"] = dst
		m.siteDetails[id] = stored
		for i := range m.sites {
			if m.sites[i].ID == id {
				m.sites[i].Name = name
			}
		}
		writeEnvelope(w, 0, "", nil)
	default:
		http.NotFound(w, r)
	}
}

// mockSiteDST validates a ModifyDstDTO and returns the DstDTO the controller
// answers a later read with. A start or end object is only acceptable with all
// of its members, which is why the empty ones from a read cannot be sent back.
func mockSiteDST(value any) (map[string]any, string) {
	update, _ := value.(map[string]any)
	stored := map[string]any{"start": map[string]any{}, "end": map[string]any{}, "status": false,
		"startTime": float64(0), "endTime": float64(0), "nextStart": float64(0), "nextEnd": float64(0), "lastStart": float64(0), "lastEnd": float64(0)}
	for field, fieldValue := range update {
		switch field {
		case "enable", "mode", "offset":
		case "start", "end":
			period, _ := fieldValue.(map[string]any)
			for _, member := range siteDSTTimeFields {
				if _, ok := period[member]; !ok {
					return nil, fmt.Sprintf("Parameter [dst.%s.%s] should not be empty", field, member)
				}
			}
		default:
			return nil, "read-only DST field: " + field
		}
		stored[field] = fieldValue
	}
	return stored, ""
}

// TestRenameSitePreservesEverythingElse guards the reason the rename is a
// read-modify-write: the endpoint replaces the whole site, so region, time
// zone, NTP, scenario and location must come back out exactly as they went in.
func TestRenameSitePreservesEverythingElse(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	m.sites = []Site{{ID: testSiteID, Name: "Default"}}
	m.siteDetails[testSiteID] = decodeTestJSON(t, testSiteDocument)
	// A field this client does not know must survive the round trip too.
	m.siteDetails[testSiteID]["futureSetting"] = "kept"
	want := decodeTestJSON(t, testSiteDocument)
	want["futureSetting"] = "kept"
	want["name"] = "example-site"
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)
	ctx := context.Background()

	if err := c.RenameSite(ctx, testSiteID, "example-site"); err != nil {
		t.Fatalf("RenameSite: %v", err)
	}

	m.mu.Lock()
	gotPath, gotBody, got := m.lastPutPath, m.lastPutBody, m.siteDetails[testSiteID]
	m.mu.Unlock()
	if wantPath := "/openapi/v1/omada1/sites/site-1"; gotPath != wantPath {
		t.Errorf("PUT path = %q, want %q", gotPath, wantPath)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("site after rename = %#v; want only the name changed: %#v", got, want)
	}
	if !reflect.DeepEqual(gotBody["dst"], map[string]any{"enable": false, "mode": float64(0)}) {
		t.Errorf("PUT dst = %#v; want only enable and mode while DST is not configured manually", gotBody["dst"])
	}
	// A setting the controller left out of the read is not invented on write.
	if tagIDs, present := gotBody["tagIds"]; present {
		t.Errorf("PUT tagIds = %#v; want the key absent, as it was read", tagIDs)
	}

	site, err := c.GetSite(ctx, testSiteID)
	if err != nil || site.Name != "example-site" {
		t.Fatalf("GetSite = %+v, %v; want the renamed site", site, err)
	}
	listed, err := c.FindSite(ctx, testSiteID)
	if err != nil || listed == nil || listed.Name != "example-site" {
		t.Fatalf("FindSite = %+v, %v; want the renamed site", listed, err)
	}
}

// TestRenameSiteKeepsManualDSTPeriod covers the one DST mode in which start,
// end and offset are configuration rather than read-side filler.
func TestRenameSiteKeepsManualDSTPeriod(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	m.siteDetails[testSiteID] = decodeTestJSON(t, testSiteDocument)
	manual := map[string]any{
		"enable": true, "mode": float64(2), "offset": float64(3600000),
		"start": map[string]any{"month": float64(3), "serial": float64(5), "day": float64(7), "hour": float64(2), "minute": float64(0)},
		"end":   map[string]any{"month": float64(10), "serial": float64(5), "day": float64(7), "hour": float64(3), "minute": float64(0)},
	}
	dst := m.siteDetails[testSiteID]["dst"].(map[string]any)
	for field, value := range manual {
		dst[field] = value
	}
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)

	if err := c.RenameSite(context.Background(), testSiteID, "example-site"); err != nil {
		t.Fatalf("RenameSite: %v", err)
	}

	m.mu.Lock()
	gotBody, got := m.lastPutBody, m.siteDetails[testSiteID]
	m.mu.Unlock()
	if !reflect.DeepEqual(gotBody["dst"], manual) {
		t.Errorf("PUT dst = %#v; want the manual period without the computed read-side fields: %#v", gotBody["dst"], manual)
	}
	stored := got["dst"].(map[string]any)
	for field, value := range manual {
		if !reflect.DeepEqual(stored[field], value) {
			t.Errorf("dst.%s after rename = %#v, want %#v", field, stored[field], value)
		}
	}
}

func TestGetSiteReportsInaccessibleSite(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)
	ctx := context.Background()

	_, err := c.GetSite(ctx, "missing")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != -1505 {
		t.Fatalf("GetSite error = %v; want API error -1505", err)
	}
	if err := c.RenameSite(ctx, "missing", "example-site"); err == nil {
		t.Fatal("RenameSite succeeded for a site that could not be read")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastPutBody != nil {
		t.Errorf("RenameSite wrote %#v although the site could not be read", m.lastPutBody)
	}
}

func TestFindSite(t *testing.T) {
	m := newMockServer(t, "6.3.0.45")
	m.sites = []Site{{ID: "site-1", Name: "Home"}, {ID: "site-2", Name: "Office"}}
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)
	ctx := context.Background()

	site, err := c.FindSite(ctx, "site-2")
	if err != nil || site == nil || site.Name != "Office" {
		t.Fatalf("FindSite by ID = %+v, %v; want Office", site, err)
	}
	// A name is not an ID: FindSite backs resource reads, which must not
	// start following a different site that happens to carry the old name.
	site, err = c.FindSite(ctx, "Office")
	if err != nil || site != nil {
		t.Fatalf("FindSite by name = %+v, %v; want nil, nil", site, err)
	}
}

// TestSiteUpdateBodyDoesNotModifyWhatWasRead keeps the read document intact,
// so that a caller can still compare against it after building the request.
func TestSiteUpdateBodyDoesNotModifyWhatWasRead(t *testing.T) {
	current := decodeTestJSON(t, testSiteDocument)
	name := "example-site"
	body := siteUpdateBody(current, SiteUpdate{Name: &name})
	if !reflect.DeepEqual(current, decodeTestJSON(t, testSiteDocument)) {
		t.Errorf("siteUpdateBody changed the document it was given: %#v", current)
	}
	if body["name"] != "example-site" {
		t.Errorf("body name = %#v, want example-site", body["name"])
	}
	for _, field := range []string{"siteId", "type"} {
		if _, ok := body[field]; ok {
			t.Errorf("body still contains the identity field %q", field)
		}
	}
}

// TestUpdateSiteChangesOnlyGivenFields covers time zone and NTP: the fields
// given are replaced, everything else survives the full-object PUT, and an
// empty server list clears the servers while a nil one keeps them.
func TestUpdateSiteChangesOnlyGivenFields(t *testing.T) {
	m := newMockServer(t, "6.3.0.111")
	m.sites = []Site{{ID: testSiteID, Name: "Default"}}
	m.siteDetails[testSiteID] = decodeTestJSON(t, testSiteDocument)
	srv := m.start()
	defer srv.Close()
	c := newTestClient(t, srv)
	ctx := context.Background()

	tz, off := "Europe/Amsterdam", false
	if err := c.UpdateSite(ctx, testSiteID, SiteUpdate{TimeZone: &tz, NTPEnable: &off}); err != nil {
		t.Fatalf("UpdateSite: %v", err)
	}
	site, err := c.GetSite(ctx, testSiteID)
	if err != nil {
		t.Fatalf("GetSite: %v", err)
	}
	if site.TimeZone != tz || site.NTPEnable || !reflect.DeepEqual(site.NTPServers, []string{"pool.ntp.org"}) || site.Name != "Default" {
		t.Errorf("site = %+v; want time zone %s, NTP off, servers and name unchanged", site, tz)
	}
	if region, _ := site.Raw["region"].(string); region != "United States" {
		t.Errorf("region = %q; want it kept", region)
	}

	if err := c.UpdateSite(ctx, testSiteID, SiteUpdate{NTPServers: []string{}}); err != nil {
		t.Fatalf("UpdateSite clearing servers: %v", err)
	}
	if site, err = c.GetSite(ctx, testSiteID); err != nil || len(site.NTPServers) != 0 {
		t.Errorf("NTP servers = %v, %v; want none after an empty list", site.NTPServers, err)
	}
}

// TestSiteDetailReadsNullNTPServers: a site that never had NTP servers
// returns null, which must read as an empty list rather than fail.
func TestSiteDetailReadsNullNTPServers(t *testing.T) {
	var site SiteDetail
	if err := json.Unmarshal([]byte(`{"name":"x","timeZone":"UTC","ntpEnable":false,"ntpServers":null}`), &site); err != nil {
		t.Fatal(err)
	}
	if site.NTPServers == nil || len(site.NTPServers) != 0 || site.TimeZone != "UTC" {
		t.Errorf("site = %+v; want an empty, non-nil server list", site)
	}
}
