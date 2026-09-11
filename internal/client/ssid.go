package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// SSID is the stable, configurable portion of an Omada wireless network.
// Raw retains complex controller-owned objects for safe read-modify-write.
type SSID struct {
	ID         string
	Name       string
	WLANID     string
	Band       int64
	Security   int64
	Broadcast  bool
	VLANEnable bool
	VLANID     int64
	Guest      bool
	Enable11r  bool
	PMFMode    int64
	Raw        map[string]any
}

// UnmarshalJSON decodes modelled fields and retains the full controller object.
func (s *SSID) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Raw = raw
	s.ID, _ = raw["id"].(string)
	s.Name, _ = raw["name"].(string)
	s.WLANID, _ = raw["wlanId"].(string)
	s.Band = int64Value(raw["band"])
	s.Security = int64Value(raw["security"])
	s.Broadcast, _ = raw["broadcast"].(bool)
	s.VLANEnable, _ = raw["vlanEnable"].(bool)
	s.VLANID = int64Value(raw["vlanId"])
	s.Guest, _ = raw["guestNetEnable"].(bool)
	s.Enable11r, _ = raw["enable11r"].(bool)
	s.PMFMode = int64Value(raw["pmfMode"])
	return nil
}

func int64Value(value any) int64 {
	if number, ok := value.(float64); ok {
		return int64(number)
	}
	return 0
}

// SSIDConfig contains fields managed by Terraform. PSK is write-only and is
// only included when explicitly configured.
type SSIDConfig struct {
	Name       string
	Band       int64
	Security   int64
	PSK        string
	Broadcast  bool
	VLANEnable bool
	VLANID     int64
	Guest      bool
	Enable11r  *bool
	PMFMode    *int64
}

func (cfg SSIDConfig) fields() map[string]any {
	fields := map[string]any{
		"name": cfg.Name, "band": cfg.Band, "security": cfg.Security,
		"broadcast": cfg.Broadcast, "vlanEnable": cfg.VLANEnable,
		"vlanId": cfg.VLANID, "guestNetEnable": cfg.Guest,
	}
	if cfg.Enable11r != nil {
		fields["enable11r"] = *cfg.Enable11r
	}
	if cfg.PMFMode != nil {
		fields["pmfMode"] = *cfg.PMFMode
	}
	return fields
}

func (c *Client) ssidsPath(siteID, groupID string) string {
	return c.classicPath(fmt.Sprintf("sites/%s/setting/wlans/%s/ssids", siteID, groupID))
}

// ListSSIDs returns every SSID in a WLAN group.
func (c *Client) ListSSIDs(ctx context.Context, siteID, groupID string) ([]SSID, error) {
	return listAllPages[SSID](ctx, c, c.ssidsPath(siteID, groupID))
}

// FindSSID finds an SSID by controller ID within a WLAN group.
func (c *Client) FindSSID(ctx context.Context, siteID, groupID, id string) (*SSID, error) {
	ssids, err := c.ListSSIDs(ctx, siteID, groupID)
	if err != nil {
		return nil, err
	}
	for _, ssid := range ssids {
		if ssid.ID == id {
			return &ssid, nil
		}
	}
	return nil, nil
}

func putSSIDPSK(fields map[string]any, psk string) {
	if psk == "" {
		return
	}
	setting, _ := fields["pskSetting"].(map[string]any)
	if setting == nil {
		setting = map[string]any{}
	}
	setting["securityKey"] = psk
	fields["pskSetting"] = setting
}

// CreateSSID creates a wireless network and resolves its generated ID by name.
func (c *Client) CreateSSID(ctx context.Context, siteID, groupID string, cfg SSIDConfig) (string, error) {
	fields := cfg.fields()
	putSSIDPSK(fields, cfg.PSK)
	if err := c.doAuthenticated(ctx, "POST", c.ssidsPath(siteID, groupID), nil, fields, nil); err != nil {
		return "", err
	}
	ssids, err := c.ListSSIDs(ctx, siteID, groupID)
	if err != nil {
		return "", err
	}
	for _, ssid := range ssids {
		if ssid.Name == cfg.Name {
			return ssid.ID, nil
		}
	}
	return "", fmt.Errorf("SSID %q was accepted but was not returned by the controller", cfg.Name)
}

// UpdateSSID overlays managed fields onto the current full object. Nested PSK
// data, including the existing password, is preserved unless a new PSK is set.
func (c *Client) UpdateSSID(ctx context.Context, siteID, groupID, id string, cfg SSIDConfig) error {
	current, err := c.FindSSID(ctx, siteID, groupID, id)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("SSID %q does not exist in WLAN group %q", id, groupID)
	}
	for key, value := range cfg.fields() {
		current.Raw[key] = value
	}
	putSSIDPSK(current.Raw, cfg.PSK)
	return c.doAuthenticated(ctx, "PATCH", fmt.Sprintf("%s/%s", c.ssidsPath(siteID, groupID), id), nil, current.Raw, nil)
}

// DeleteSSID removes a wireless network from a WLAN group.
func (c *Client) DeleteSSID(ctx context.Context, siteID, groupID, id string) error {
	return c.doAuthenticated(ctx, "DELETE", fmt.Sprintf("%s/%s", c.ssidsPath(siteID, groupID), id), nil, nil, nil)
}
