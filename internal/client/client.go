// Package client implements a minimal HTTP client for a TP-Link Omada
// Controller's local (non-cloud) management API: the /api/info + /login
// handshake, session-expiry retry, and the small set of endpoints the
// omada_dhcp_reservation resource needs.
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
)

// Client is an authenticated HTTP client for a single Omada Controller.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client

	mu            sync.Mutex
	omadacID      string
	controllerVer string
	token         string
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

// NewClient builds a Client for the given controller base URL. Call
// Authenticate before making any other request.
func NewClient(baseURL, username, password string, skipTLSVerify bool) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	transport := &http.Transport{}
	if skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit opt-in via provider config
	}

	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		httpClient: &http.Client{
			Jar:       jar,
			Transport: transport,
		},
	}, nil
}

// Authenticate fetches controller info and logs in, populating the omadacId,
// controller version, and session token every later request needs.
func (c *Client) Authenticate(ctx context.Context) error {
	if err := c.fetchInfo(ctx); err != nil {
		return err
	}
	return c.login(ctx)
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

	resp, err := c.httpClient.Do(req)
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
// result into out (when non-nil). A session-expired response (errorCode
// -1) triggers exactly one re-login and retry of the same call.
func (c *Client) doAuthenticated(ctx context.Context, method, path string, query url.Values, body, out any) error {
	return c.doAuthenticatedWithHeaders(ctx, method, path, query, body, out, nil)
}

func (c *Client) doAuthenticatedWithHeaders(ctx context.Context, method, path string, query url.Values, body, out any, headers map[string]string) error {
	return c.doAuthenticatedAttempt(ctx, method, path, query, body, out, headers, true)
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

	if env.ErrorCode == -1 && allowRetry {
		if err := c.login(ctx); err != nil {
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
