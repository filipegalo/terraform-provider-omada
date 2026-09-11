package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const lanNetworksEndpoint = "lan-networks"

var openAPIHeaders = map[string]string{"Omada-Request-Source": "web-local"}

// VLAN is an Omada LAN interface VLAN. Raw is retained for fields the
// provider does not own; writes use the API's known writable shape.
type VLAN struct {
	ID             string
	Name           string
	VLANID         int64
	Primary        bool
	InterfaceIDs   []string
	DeviceMAC      string
	GatewaySubnet  string
	Isolation      bool
	DHCPEnabled    bool
	DHCPRangeStart string
	DHCPRangeEnd   string
	DHCPDNSMode    string
	DHCPPrimaryDNS string
	DHCPLeaseTime  int64
	Raw            map[string]any
}

// UnmarshalJSON keeps the raw object alongside the fields the provider
// models, so writes can preserve what it does not own.
func (v *VLAN) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v.Raw = raw
	v.ID, _ = raw["id"].(string)
	v.Name, _ = raw["name"].(string)
	if value, ok := raw["vlan"].(float64); ok {
		v.VLANID = int64(value)
	}
	v.Primary, _ = raw["primary"].(bool)
	if ids, ok := raw["interfaceIds"].([]any); ok {
		for _, id := range ids {
			if s, ok := id.(string); ok {
				v.InterfaceIDs = append(v.InterfaceIDs, s)
			}
		}
	}
	v.DeviceMAC, _ = raw["deviceMac"].(string)
	v.GatewaySubnet, _ = raw["gatewaySubnet"].(string)
	v.Isolation, _ = raw["isolation"].(bool)
	v.parseDHCPSettings(raw)
	return nil
}

// parseDHCPSettings reads the nested DHCP scope so an imported VLAN hydrates
// every attribute the resource models, not just its identity.
func (v *VLAN) parseDHCPSettings(raw map[string]any) {
	dhcp, ok := raw["dhcpSettings"].(map[string]any)
	if !ok {
		return
	}
	v.DHCPEnabled, _ = dhcp["enable"].(bool)
	v.DHCPDNSMode, _ = dhcp["dhcpns"].(string)
	v.DHCPPrimaryDNS, _ = dhcp["priDns"].(string)
	if value, ok := dhcp["leasetime"].(float64); ok {
		v.DHCPLeaseTime = int64(value)
	}
	pool, ok := dhcp["ipRangePool"].([]any)
	if !ok || len(pool) == 0 {
		return
	}
	if first, ok := pool[0].(map[string]any); ok {
		v.DHCPRangeStart, _ = first["ipaddrStart"].(string)
		v.DHCPRangeEnd, _ = first["ipaddrEnd"].(string)
	}
}

// VLANConfig is the writable LAN-network body confirmed by the controller's
// POST .../networks/param-check endpoint.
type VLANConfig struct {
	Name               string
	DeviceMAC          string
	DeviceType         int64
	VLANType           int64
	VLANID             int64
	GatewaySubnet      string
	DHCPEnabled        bool
	DHCPRangeStart     string
	DHCPRangeEnd       string
	DHCPDNSMode        string
	DHCPPrimaryDNS     string
	DHCPLeaseTime      int64
	Isolation          bool
	InterfaceIDs       []string
	IGMPSnoopEnable    bool
	MLDSnoopEnable     bool
	FastLeaveEnable    bool
	DHCPGuardEnable    bool
	DHCPv6GuardEnable  bool
	DHCPL2RelayEnable  bool
	ARPDetectionEnable bool
	QoSQueueEnable     bool
}

func (cfg VLANConfig) requestBody() map[string]any {
	dhcp := map[string]any{"enable": cfg.DHCPEnabled, "dhcpns": cfg.DHCPDNSMode, "leasetime": cfg.DHCPLeaseTime, "gatewayMode": "auto", "options": []any{}, "dhcpPoolMask": 24}
	if cfg.DHCPEnabled {
		dhcp["ipRangePool"] = []map[string]string{{"ipaddrStart": cfg.DHCPRangeStart, "ipaddrEnd": cfg.DHCPRangeEnd}}
	}
	if cfg.DHCPDNSMode == "manual" && cfg.DHCPPrimaryDNS != "" {
		dhcp["priDns"] = cfg.DHCPPrimaryDNS
	}
	return map[string]any{
		// purpose 1 marks a gateway LAN interface VLAN, the only kind this
		// resource manages; the controller rejects the body without it.
		"purpose": 1, "interfaceIds": cfg.InterfaceIDs,
		"name": cfg.Name, "deviceMac": cfg.DeviceMAC, "deviceType": cfg.DeviceType, "vlanType": cfg.VLANType, "vlan": cfg.VLANID, "gatewaySubnet": cfg.GatewaySubnet, "dhcpSettings": dhcp,
		"upnpLanEnable": false, "igmpSnoopEnable": cfg.IGMPSnoopEnable, "dhcpGuard": map[string]bool{"enable": cfg.DHCPGuardEnable}, "dhcpv6Guard": map[string]bool{"enable": cfg.DHCPv6GuardEnable},
		"lanNetworkIpv6Config": map[string]int{"proto": 0, "enable": 0}, "qosQueueEnable": cfg.QoSQueueEnable, "isolation": cfg.Isolation, "mldSnoopEnable": cfg.MLDSnoopEnable,
		"fastLeaveEnable": cfg.FastLeaveEnable, "arpDetectionEnable": cfg.ARPDetectionEnable, "dhcpL2RelayEnable": cfg.DHCPL2RelayEnable,
	}
}

func (c *Client) lanNetworksPath(version, siteID string) string {
	return c.openAPIPath(version, siteID, lanNetworksEndpoint)
}

// ListVLANs reads the current OpenAPI v3 endpoint. purpose is numeric on
// this surface (unlike the old classic API), so all LAN interface VLANs are
// returned instead of filtering on an obsolete string value.
func (c *Client) ListVLANs(ctx context.Context, siteID string) ([]VLAN, error) {
	var response struct {
		Data []VLAN `json:"data"`
	}
	q := url.Values{"page": {"1"}, "pageSize": {"100"}, "searchKey": {""}}
	if err := c.doAuthenticatedWithHeaders(ctx, "GET", c.lanNetworksPath("v3", siteID), q, nil, &response, openAPIHeaders); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// FindVLAN returns the LAN network with the given ID, or nil when the site
// has no such network.
func (c *Client) FindVLAN(ctx context.Context, siteID, id string) (*VLAN, error) {
	vlans, err := c.ListVLANs(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for _, vlan := range vlans {
		if vlan.ID == id {
			return &vlan, nil
		}
	}
	return nil, nil
}

// gatewayLANInterfaceIDs returns the LAN interfaces a new VLAN should bind to.
// The controller rejects a LAN network with no interfaces (API error -33515),
// and the only surface that enumerates every LAN port is the set already in
// use by the gateway's own LAN networks -- setting/wan/networks lists just the
// WAN-capable ones. The primary (default) network is preferred because it is
// always present and spans every LAN port, matching the controller UI's
// default selection for a new network.
func (c *Client) gatewayLANInterfaceIDs(ctx context.Context, siteID, deviceMAC string) ([]string, error) {
	vlans, err := c.ListVLANs(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("listing LAN networks to resolve gateway interfaces: %w", err)
	}
	var fallback []string
	for _, vlan := range vlans {
		mac, _ := vlan.Raw["deviceMac"].(string)
		if !strings.EqualFold(mac, deviceMAC) || len(vlan.InterfaceIDs) == 0 {
			continue
		}
		if vlan.Primary {
			return vlan.InterfaceIDs, nil
		}
		if fallback == nil {
			fallback = vlan.InterfaceIDs
		}
	}
	if fallback == nil {
		return nil, fmt.Errorf("gateway %s has no existing LAN network to take LAN interfaces from", deviceMAC)
	}
	return fallback, nil
}

// CreateVLAN validates the exact body with param-check before issuing the
// mutation, preventing an incomplete request from changing the network.
func (c *Client) CreateVLAN(ctx context.Context, siteID string, cfg VLANConfig) (string, error) {
	if len(cfg.InterfaceIDs) == 0 {
		ids, err := c.gatewayLANInterfaceIDs(ctx, siteID, cfg.DeviceMAC)
		if err != nil {
			return "", err
		}
		cfg.InterfaceIDs = ids
	}
	if err := c.paramCheckVLAN(ctx, siteID, cfg); err != nil {
		return "", fmt.Errorf("validating VLAN parameters: %w", err)
	}
	body := cfg.requestBody()
	if err := c.doAuthenticatedWithHeaders(ctx, "POST", c.lanNetworksPath("v1", siteID), nil, body, nil, openAPIHeaders); err != nil {
		return "", err
	}
	vlans, err := c.ListVLANs(ctx, siteID)
	if err != nil {
		return "", fmt.Errorf("listing VLAN after creation: %w", err)
	}
	for _, vlan := range vlans {
		if vlan.Name == cfg.Name && vlan.VLANID == cfg.VLANID {
			return vlan.ID, nil
		}
	}
	return "", fmt.Errorf("VLAN %q (tag %d) was accepted but was not returned by the controller", cfg.Name, cfg.VLANID)
}

// UpdateVLAN preserves the VLAN's current LAN interfaces, so an update does
// not silently rebind it to a different set of gateway ports.
func (c *Client) UpdateVLAN(ctx context.Context, siteID, id string, cfg VLANConfig) error {
	if len(cfg.InterfaceIDs) == 0 {
		existing, err := c.FindVLAN(ctx, siteID, id)
		if err != nil {
			return err
		}
		if existing != nil {
			cfg.InterfaceIDs = existing.InterfaceIDs
		}
	}
	return c.doAuthenticatedWithHeaders(ctx, "PUT", fmt.Sprintf("%s/%s", c.lanNetworksPath("v1", siteID), id), nil, cfg.requestBody(), nil, openAPIHeaders)
}

// DeleteVLAN removes a LAN network from the site.
func (c *Client) DeleteVLAN(ctx context.Context, siteID, id string) error {
	return c.doAuthenticatedWithHeaders(ctx, "DELETE", fmt.Sprintf("%s/%s", c.lanNetworksPath("v1", siteID), id), nil, nil, nil, openAPIHeaders)
}

// paramCheckVLAN validates a VLAN body against the controller without
// mutating anything.
func (c *Client) paramCheckVLAN(ctx context.Context, siteID string, cfg VLANConfig) error {
	checkPath := fmt.Sprintf("%s/param-check", c.openAPIPath("v1", siteID, "networks"))
	return c.doAuthenticatedWithHeaders(ctx, "POST", checkPath, nil, cfg.requestBody(), nil, openAPIHeaders)
}
