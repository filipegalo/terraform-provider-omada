package client

import (
	"context"
	"fmt"
	"strings"
)

// SwitchPort is the configurable part of a physical switch port. The controller
// adds runtime counters and capabilities which must never be written back.
type SwitchPort struct {
	ID                        string   `json:"id"`
	Port                      int64    `json:"port"`
	SwitchMAC                 string   `json:"switchMac"`
	Name                      string   `json:"name"`
	TagIDs                    []string `json:"tagIds"`
	NativeNetworkID           string   `json:"nativeNetworkId"`
	NetworkTagsSetting        int64    `json:"networkTagsSetting"`
	ProfileID                 string   `json:"profileId"`
	ProfileOverrideEnable     bool     `json:"profileOverrideEnable"`
	ProfileVLANOverrideEnable bool     `json:"profileVlanOverrideEnable"`
	LinkSpeed                 int64    `json:"linkSpeed"`
	Duplex                    int64    `json:"duplex"`
}

// SwitchPortConfig is the allow-list accepted by the controller's OpenAPI
// PATCH endpoint. Never round-trip the raw web API document: it contains
// telemetry and capabilities that the PATCH endpoint rejects.
type SwitchPortConfig struct {
	Name                      string   `json:"name"`
	TagIDs                    []string `json:"tagIds"`
	NativeNetworkID           string   `json:"nativeNetworkId"`
	NetworkTagsSetting        int64    `json:"networkTagsSetting"`
	ProfileID                 string   `json:"profileId"`
	ProfileOverrideEnable     bool     `json:"profileOverrideEnable"`
	ProfileVLANOverrideEnable bool     `json:"profileVlanOverrideEnable"`
	LinkSpeed                 int64    `json:"linkSpeed"`
	Duplex                    int64    `json:"duplex"`
}

func (c *Client) switchPortsPath(siteID, switchMAC string) string {
	return c.classicPath(fmt.Sprintf("sites/%s/switches/%s/ports", siteID, NormalizeSwitchMAC(switchMAC)))
}

// ListSwitchPorts uses the web API because it is the authenticated endpoint
// which exposes the full current port configuration. Its result is a bare
// array, unlike the paginated {data: ...} response used by many endpoints.
func (c *Client) ListSwitchPorts(ctx context.Context, siteID, switchMAC string) ([]SwitchPort, error) {
	var ports []SwitchPort
	if err := c.doAuthenticated(ctx, "GET", c.switchPortsPath(siteID, switchMAC), nil, nil, &ports); err != nil {
		return nil, err
	}
	return ports, nil
}

// FindSwitchPort finds a physical port by its number, not its opaque object
// ID: the controller rejects the object ID in the PATCH URL.
func (c *Client) FindSwitchPort(ctx context.Context, siteID, switchMAC string, port int64) (*SwitchPort, error) {
	ports, err := c.ListSwitchPorts(ctx, siteID, switchMAC)
	if err != nil {
		return nil, err
	}
	for _, candidate := range ports {
		if candidate.Port == port {
			return &candidate, nil
		}
	}
	return nil, nil
}

// UpdateSwitchPort writes the small known-safe request body to the OpenAPI
// mirror of the web endpoint.
func (c *Client) UpdateSwitchPort(ctx context.Context, siteID, switchMAC string, port int64, cfg SwitchPortConfig) error {
	path := c.openAPIPath("v1", siteID, fmt.Sprintf("switches/%s/ports/%d", NormalizeSwitchMAC(switchMAC), port))
	return c.doAuthenticatedWithHeaders(ctx, "PATCH", path, nil, cfg, nil, openAPIHeaders)
}

// NormalizeSwitchMAC returns the dash-separated uppercase representation the
// controller expects in endpoint paths. Resource state still preserves the
// user's spelling, because changing a configured value after planning would
// violate Terraform's state-consistency contract.
func NormalizeSwitchMAC(mac string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(mac), ":", "-"))
}
