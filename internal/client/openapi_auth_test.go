package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAPIClientCredentialsAuthentication(t *testing.T) {
	var oauthLogins, openAPICalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			writeEnvelope(w, 0, "", map[string]any{"omadacId": testOmadacID, "controllerVer": "6.3.0.45"})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
			writeEnvelope(w, 0, "", map[string]any{"token": "classic-token"})
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/authorize/token":
			if r.URL.Query().Get("grant_type") != "client_credentials" {
				t.Errorf("grant_type = %q, want client_credentials", r.URL.Query().Get("grant_type"))
			}
			oauthLogins++
			writeEnvelope(w, 0, "", map[string]any{"accessToken": "oauth-token", "expiresIn": 7200})
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/openapi/v1/%s/sites/site-1/probe", testOmadacID):
			openAPICalls++
			if got := r.Header.Get("Authorization"); got != "AccessToken=oauth-token" {
				t.Errorf("Authorization = %q, want AccessToken=oauth-token", got)
			}
			if got := r.Header.Get("Csrf-Token"); got != "" {
				t.Errorf("Csrf-Token = %q, want empty with Open API auth", got)
			}
			if got := r.URL.Query().Get("token"); got != "" {
				t.Errorf("token query = %q, want empty with Open API auth", got)
			}
			writeEnvelope(w, 0, "", map[string]any{"ok": true})
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
	if err := c.doAuthenticated(ctx, http.MethodGet, c.openAPIPath("v1", "site-1", "probe"), nil, nil, nil); err != nil {
		t.Fatalf("Open API request: %v", err)
	}
	if oauthLogins != 1 || openAPICalls != 1 {
		t.Fatalf("oauthLogins=%d openAPICalls=%d, want 1 and 1", oauthLogins, openAPICalls)
	}
}

func TestOpenAPITokenExpiryRefreshesAndRetriesOnce(t *testing.T) {
	var oauthLogins, openAPICalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			writeEnvelope(w, 0, "", map[string]any{"omadacId": testOmadacID, "controllerVer": "6.3.0.45"})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
			writeEnvelope(w, 0, "", map[string]any{"token": "classic-token"})
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/authorize/token":
			oauthLogins++
			writeEnvelope(w, 0, "", map[string]any{"accessToken": fmt.Sprintf("oauth-%d", oauthLogins), "expiresIn": 7200})
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/openapi/v1/%s/sites/site-1/probe", testOmadacID):
			openAPICalls++
			if openAPICalls == 1 {
				writeEnvelope(w, -44112, "access token expired", nil)
				return
			}
			if got := r.Header.Get("Authorization"); got != "AccessToken=oauth-2" {
				t.Errorf("retry Authorization = %q, want AccessToken=oauth-2", got)
			}
			writeEnvelope(w, 0, "", nil)
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
	if err := c.doAuthenticated(ctx, http.MethodGet, c.openAPIPath("v1", "site-1", "probe"), nil, nil, nil); err != nil {
		t.Fatalf("Open API request: %v", err)
	}
	if oauthLogins != 2 || openAPICalls != 2 {
		t.Fatalf("oauthLogins=%d openAPICalls=%d, want 2 and 2", oauthLogins, openAPICalls)
	}
}

func TestOpenAPIProactivelyRefreshesNearExpiry(t *testing.T) {
	var oauthLogins int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			writeEnvelope(w, 0, "", map[string]any{"omadacId": testOmadacID, "controllerVer": "6.3.0.45"})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
			writeEnvelope(w, 0, "", map[string]any{"token": "classic-token"})
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/authorize/token":
			oauthLogins++
			writeEnvelope(w, 0, "", map[string]any{"accessToken": fmt.Sprintf("oauth-%d", oauthLogins), "expiresIn": 7200})
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/openapi/v1/%s/sites/site-1/probe", testOmadacID):
			if got := r.Header.Get("Authorization"); got != "AccessToken=oauth-2" {
				t.Errorf("Authorization = %q, want proactively refreshed oauth-2", got)
			}
			writeEnvelope(w, 0, "", nil)
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
	c.mu.Lock()
	c.accessTokenExpiresAt = time.Now().Add(30 * time.Second)
	c.mu.Unlock()
	if err := c.doAuthenticated(ctx, http.MethodGet, c.openAPIPath("v1", "site-1", "probe"), nil, nil, nil); err != nil {
		t.Fatalf("Open API request: %v", err)
	}
	if oauthLogins != 2 {
		t.Fatalf("oauthLogins=%d, want 2", oauthLogins)
	}
}

func TestOpenAPIFallsBackToClassicAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			writeEnvelope(w, 0, "", map[string]any{"omadacId": testOmadacID, "controllerVer": "6.3.0.45"})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
			writeEnvelope(w, 0, "", map[string]any{"token": "classic-token"})
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/openapi/v1/%s/sites/site-1/probe", testOmadacID):
			if r.Header.Get("Csrf-Token") != "classic-token" || r.URL.Query().Get("token") != "classic-token" {
				t.Error("Open API fallback did not use the classic token and CSRF header")
			}
			if r.Header.Get("Authorization") != "" {
				t.Error("classic fallback unexpectedly sent an Authorization header")
			}
			writeEnvelope(w, 0, "", nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c, err := NewClient(server.URL, "admin", "password", "", "", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()
	if err := c.Authenticate(ctx); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if err := c.doAuthenticated(ctx, http.MethodGet, c.openAPIPath("v1", "site-1", "probe"), nil, nil, nil); err != nil {
		t.Fatalf("Open API request with classic fallback: %v", err)
	}
}

func TestInternalWebOpenAPIPathUsesClassicAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			writeEnvelope(w, 0, "", map[string]any{"omadacId": testOmadacID, "controllerVer": "6.3.0.45"})
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/authorize/token":
			writeEnvelope(w, 0, "", map[string]any{"accessToken": "oauth-token", "expiresIn": 7200})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
			writeEnvelope(w, 0, "", map[string]any{"token": "classic-token"})
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/openapi/v3/%s/sites/site-1/lan-networks", testOmadacID):
			if got := r.Header.Get("Csrf-Token"); got != "classic-token" {
				t.Errorf("Csrf-Token = %q, want classic-token", got)
			}
			if got := r.URL.Query().Get("token"); got != "classic-token" {
				t.Errorf("token query = %q, want classic-token", got)
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("Authorization = %q, want empty for internal web API", got)
			}
			writeEnvelope(w, 0, "", map[string]any{"data": []VLAN{}})
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
	if _, err := c.ListVLANs(ctx, "site-1"); err != nil {
		t.Fatalf("ListVLANs: %v", err)
	}
}

func TestOpenAPICookiesDoNotReplaceClassicSession(t *testing.T) {
	var classicLogins int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			writeEnvelope(w, 0, "", map[string]any{"omadacId": testOmadacID, "controllerVer": "6.3.0.45"})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
			classicLogins++
			http.SetCookie(w, &http.Cookie{Name: "TPOMADA_SESSIONID", Value: "classic-session", Path: "/"})
			writeEnvelope(w, 0, "", map[string]any{"token": "classic-token"})
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/authorize/token":
			http.SetCookie(w, &http.Cookie{Name: "TPOMADA_SESSIONID", Value: "openapi-session", Path: "/"})
			writeEnvelope(w, 0, "", map[string]any{"accessToken": "oauth-token", "expiresIn": 7200})
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/%s/api/v2/sites", testOmadacID):
			cookie, err := r.Cookie("TPOMADA_SESSIONID")
			if err != nil || cookie.Value != "classic-session" {
				writeEnvelope(w, -1200, "logged out", nil)
				return
			}
			writeEnvelope(w, 0, "", pageResult[Site]{CurrentPage: 1, CurrentSize: 0, TotalRows: 0, Data: []Site{}})
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
	if _, err := c.ListSites(ctx); err != nil {
		t.Fatalf("classic request after Open API authentication: %v", err)
	}
	if classicLogins != 1 {
		t.Fatalf("classicLogins=%d, want 1", classicLogins)
	}
}

func TestClassicLoggedOutErrorRetriesOnce(t *testing.T) {
	var logins, calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			writeEnvelope(w, 0, "", map[string]any{"omadacId": testOmadacID, "controllerVer": "6.3.0.45"})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/%s/api/v2/login", testOmadacID):
			logins++
			writeEnvelope(w, 0, "", map[string]any{"token": fmt.Sprintf("classic-%d", logins)})
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/%s/api/v2/sites", testOmadacID):
			calls++
			if calls == 1 {
				writeEnvelope(w, -1200, "logged out", nil)
				return
			}
			writeEnvelope(w, 0, "", pageResult[Site]{CurrentPage: 1, CurrentSize: 0, TotalRows: 0, Data: []Site{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c, err := NewClient(server.URL, "admin", "password", "", "", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()
	if err := c.Authenticate(ctx); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if _, err := c.ListSites(ctx); err != nil {
		t.Fatalf("classic request retry: %v", err)
	}
	if logins != 2 || calls != 2 {
		t.Fatalf("logins=%d calls=%d, want 2 and 2", logins, calls)
	}
}

func TestNewClientRejectsPartialOpenAPICredentials(t *testing.T) {
	if _, err := NewClient("https://controller.example", "admin", "password", "client-id", "", false); err == nil {
		t.Fatal("NewClient accepted an Open API client ID without a client secret")
	}
	if _, err := NewClient("https://controller.example", "admin", "password", "", "client-secret", false); err == nil {
		t.Fatal("NewClient accepted an Open API client secret without a client ID")
	}
}
