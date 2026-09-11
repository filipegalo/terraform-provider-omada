// Package client implements authenticated access to a TP-Link Omada
// Controller's classic and Open APIs, including token renewal and the
// controller-specific request envelopes used by both surfaces.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client is an authenticated HTTP client for a single Omada Controller.
type Client struct {
	baseURL       string
	username      string
	password      string
	clientID      string
	clientSecret  string
	httpClient    *http.Client
	openAPIClient *http.Client

	mu                   sync.Mutex
	authMu               sync.Mutex
	omadacID             string
	controllerVer        string
	token                string
	accessToken          string
	accessTokenExpiresAt time.Time
}

// envelope is the response shape the controller wraps every API response
// in, authenticated or not.
type envelope struct {
	ErrorCode int             `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

type infoResult struct {
	OmadacID      string `json:"omadacId"`
	ControllerVer string `json:"controllerVer"`
}

type loginResult struct {
	Token string `json:"token"`
}

type openAPITokenResult struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int64  `json:"expiresIn"`
}

// NewClient builds a Client for the given controller base URL. Call
// Authenticate before making any other request.
func NewClient(baseURL, username, password, clientID, clientSecret string, skipTLSVerify bool) (*Client, error) {
	if (clientID == "") != (clientSecret == "") {
		return nil, fmt.Errorf("open API client ID and client secret must be configured together")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}
	openAPIJar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating Open API cookie jar: %w", err)
	}

	transport := &http.Transport{}
	if skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit opt-in via provider config
	}

	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		username:     username,
		password:     password,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Jar:       jar,
			Transport: transport,
		},
		openAPIClient: &http.Client{
			Jar:       openAPIJar,
			Transport: transport,
		},
	}, nil
}

// Authenticate fetches controller info, establishes the classic session, and
// obtains an Open API access token when client credentials are configured.
func (c *Client) Authenticate(ctx context.Context) error {
	if err := c.fetchInfo(ctx); err != nil {
		return err
	}
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if err := c.login(ctx); err != nil {
		return err
	}
	if c.hasOpenAPICredentials() {
		if err := c.loginOpenAPI(ctx); err != nil {
			return fmt.Errorf("authenticating with Open API client credentials: %w", err)
		}
	}
	return nil
}

func (c *Client) hasOpenAPICredentials() bool {
	return c.clientID != "" && c.clientSecret != ""
}

func (c *Client) fetchInfo(ctx context.Context) error {
	env, err := c.doRaw(ctx, http.MethodGet, "/api/info", nil, nil, nil)
	if err != nil {
		return err
	}
	if err := checkEnvelope(env); err != nil {
		return err
	}

	var info infoResult
	if err := json.Unmarshal(env.Result, &info); err != nil {
		return fmt.Errorf("decoding /api/info result: %w", err)
	}

	c.mu.Lock()
	c.omadacID = info.OmadacID
	c.controllerVer = info.ControllerVer
	c.mu.Unlock()
	return nil
}

func (c *Client) login(ctx context.Context) error {
	c.mu.Lock()
	omadacID := c.omadacID
	c.mu.Unlock()

	body := map[string]string{
		"username": c.username,
		"password": c.password,
	}

	env, err := c.doRaw(ctx, http.MethodPost, fmt.Sprintf("/%s/api/v2/login", omadacID), nil, body, nil)
	if err != nil {
		return err
	}
	if err := checkEnvelope(env); err != nil {
		return err
	}

	var res loginResult
	if err := json.Unmarshal(env.Result, &res); err != nil {
		return fmt.Errorf("decoding login result: %w", err)
	}

	c.mu.Lock()
	c.token = res.Token
	c.mu.Unlock()
	return nil
}

func (c *Client) loginOpenAPI(ctx context.Context) error {
	c.mu.Lock()
	omadacID := c.omadacID
	c.mu.Unlock()
	body := map[string]string{
		"omadacId":      omadacID,
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
	}
	query := url.Values{"grant_type": {"client_credentials"}}
	env, err := c.doRaw(ctx, http.MethodPost, "/openapi/authorize/token", query, body, nil)
	if err != nil {
		return err
	}
	if err := checkEnvelope(env); err != nil {
		return err
	}
	var res openAPITokenResult
	if err := json.Unmarshal(env.Result, &res); err != nil {
		return fmt.Errorf("decoding Open API token result: %w", err)
	}
	if res.AccessToken == "" {
		return fmt.Errorf("open API token response did not contain an accessToken")
	}
	expiresIn := time.Duration(res.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 2 * time.Hour
	}
	c.mu.Lock()
	c.accessToken = res.AccessToken
	c.accessTokenExpiresAt = time.Now().Add(expiresIn)
	c.mu.Unlock()
	return nil
}

// doRaw performs a single HTTP call and decodes the response envelope. It
// does not interpret errorCode: callers decide what a non-zero code means.
func (c *Client) doRaw(ctx context.Context, method, path string, query url.Values, body any, headers map[string]string) (*envelope, error) {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	httpClient := c.httpClient
	// The controller can set a session cookie while issuing an Open API token.
	// Keep Open API cookies isolated so that they cannot replace the classic
	// controller session used by resources that still rely on /api/v2.
	if c.hasOpenAPICredentials() && (path == "/openapi/authorize/token" || strings.HasPrefix(headers["Authorization"], "AccessToken=")) {
		httpClient = c.openAPIClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("unexpected HTTP status %d: %s", resp.StatusCode, string(data))
	}

	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decoding response envelope: %w", err)
	}
	return &env, nil
}

func checkEnvelope(env *envelope) error {
	if env.ErrorCode != 0 {
		return fmt.Errorf("API error %d: %s", env.ErrorCode, env.Msg)
	}
	return nil
}

// doAuthenticated performs a token-authenticated request, decoding the
// result into out (when non-nil). Authentication-expired responses trigger
// exactly one refresh and retry of the same call.
func (c *Client) doAuthenticated(ctx context.Context, method, path string, query url.Values, body, out any) error {
	return c.doAuthenticatedWithHeaders(ctx, method, path, query, body, out, nil)
}

func (c *Client) doAuthenticatedWithHeaders(ctx context.Context, method, path string, query url.Values, body, out any, headers map[string]string) error {
	// Omada also exposes controller-internal web endpoints below /openapi.
	// Requests marked web-local require the classic session token even when
	// client credentials are configured; only the public Open API accepts the
	// AccessToken authorization scheme.
	usesInternalWebAPI := strings.EqualFold(headers["Omada-Request-Source"], "web-local")
	if strings.HasPrefix(path, "/openapi/") && !usesInternalWebAPI && c.hasOpenAPICredentials() {
		return c.doOpenAPIAttempt(ctx, method, path, query, body, out, headers, true)
	}
	return c.doAuthenticatedAttempt(ctx, method, path, query, body, out, headers, true)
}

func (c *Client) doOpenAPIAttempt(ctx context.Context, method, path string, query url.Values, body, out any, headers map[string]string, allowRetry bool) error {
	accessToken, err := c.ensureOpenAPIToken(ctx)
	if err != nil {
		return err
	}
	requestHeaders := make(map[string]string, len(headers)+1)
	for key, value := range headers {
		requestHeaders[key] = value
	}
	requestHeaders["Authorization"] = "AccessToken=" + accessToken
	env, err := c.doRaw(ctx, method, path, query, body, requestHeaders)
	if err != nil {
		return err
	}
	if isOpenAPIAuthError(env.ErrorCode) && allowRetry {
		if err := c.refreshOpenAPIToken(ctx, accessToken); err != nil {
			return fmt.Errorf("re-authenticating after Open API token expiry: %w", err)
		}
		return c.doOpenAPIAttempt(ctx, method, path, query, body, out, headers, false)
	}
	if err := checkEnvelope(env); err != nil {
		return err
	}
	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("decoding result: %w", err)
		}
	}
	return nil
}

func (c *Client) ensureOpenAPIToken(ctx context.Context) (string, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	c.mu.Lock()
	accessToken := c.accessToken
	expiresAt := c.accessTokenExpiresAt
	c.mu.Unlock()
	if accessToken != "" && time.Until(expiresAt) > time.Minute {
		return accessToken, nil
	}
	if err := c.loginOpenAPI(ctx); err != nil {
		return "", fmt.Errorf("refreshing Open API access token: %w", err)
	}
	c.mu.Lock()
	accessToken = c.accessToken
	c.mu.Unlock()
	return accessToken, nil
}

func (c *Client) refreshOpenAPIToken(ctx context.Context, staleToken string) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	c.mu.Lock()
	currentToken := c.accessToken
	c.mu.Unlock()
	if currentToken != staleToken {
		return nil
	}
	if err := c.loginOpenAPI(ctx); err != nil {
		return err
	}
	return nil
}

func isOpenAPIAuthError(code int) bool {
	switch code {
	case -44112, -44113, -44116:
		return true
	default:
		return false
	}
}

func (c *Client) doAuthenticatedAttempt(ctx context.Context, method, path string, query url.Values, body, out any, headers map[string]string, allowRetry bool) error {
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()

	q := url.Values{}
	for k, vs := range query {
		q[k] = vs
	}
	q.Set("token", token)

	requestHeaders := make(map[string]string, len(headers)+1)
	for key, value := range headers {
		requestHeaders[key] = value
	}
	requestHeaders["Csrf-Token"] = token
	env, err := c.doRaw(ctx, method, path, q, body, requestHeaders)
	if err != nil {
		return err
	}

	if isClassicAuthError(env.ErrorCode) && allowRetry {
		if err := c.refreshClassicSession(ctx, token); err != nil {
			return fmt.Errorf("re-authenticating after session expiry: %w", err)
		}
		return c.doAuthenticatedAttempt(ctx, method, path, query, body, out, headers, false)
	}

	if err := checkEnvelope(env); err != nil {
		return err
	}

	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("decoding result: %w", err)
		}
	}
	return nil
}

func (c *Client) refreshClassicSession(ctx context.Context, staleToken string) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	c.mu.Lock()
	currentToken := c.token
	c.mu.Unlock()
	if currentToken != staleToken {
		return nil
	}
	return c.login(ctx)
}

func isClassicAuthError(code int) bool {
	return code == -1 || code == -1200
}

func (c *Client) openAPIPath(version, siteID, endpoint string) string {
	c.mu.Lock()
	omadacID := c.omadacID
	c.mu.Unlock()
	return fmt.Sprintf("/openapi/%s/%s/sites/%s/%s", version, omadacID, siteID, endpoint)
}

// classicPath builds a path under {base_url}/{omadacId}/api/v2/{endpoint}.
func (c *Client) classicPath(endpoint string) string {
	c.mu.Lock()
	omadacID := c.omadacID
	c.mu.Unlock()
	return fmt.Sprintf("/%s/api/v2/%s", omadacID, endpoint)
}
