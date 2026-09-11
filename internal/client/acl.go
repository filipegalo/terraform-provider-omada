package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

const (
	// ACLTypeGateway selects gateway ACL rules.
	ACLTypeGateway int64 = 0
	// ACLTypeSwitch selects switch ACL rules.
	ACLTypeSwitch int64 = 1
	// ACLTypeEAP selects access-point ACL rules.
	ACLTypeEAP int64 = 2
)

// ACLDirection is the gateway traffic direction encoded by Omada.
type ACLDirection struct {
	LANToWAN bool
	LANToLAN bool
	WANInIDs []string
	VPNInIDs []string
}

// ACL is a firewall rule. Raw is retained because updates are full-object PUTs.
type ACL struct {
	ID              string
	Type            int64
	Name            string
	Enabled         bool
	Policy          int64
	Protocols       []int64
	SourceType      int64
	SourceIDs       []string
	DestinationType int64
	DestinationIDs  []string
	Direction       ACLDirection
	Raw             map[string]any
}

// UnmarshalJSON decodes modelled ACL fields and retains the complete rule.
func (a *ACL) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Raw = raw
	a.ID, _ = raw["id"].(string)
	a.Type = int64Value(raw["type"])
	a.Name, _ = raw["name"].(string)
	a.Enabled, _ = raw["status"].(bool)
	a.Policy = int64Value(raw["policy"])
	a.Protocols = int64Values(raw["protocols"])
	a.SourceType = int64Value(raw["sourceType"])
	a.SourceIDs = stringValues(raw["sourceIds"])
	a.DestinationType = int64Value(raw["destinationType"])
	a.DestinationIDs = stringValues(raw["destinationIds"])
	if direction, ok := raw["direction"].(map[string]any); ok {
		a.Direction.LANToWAN, _ = direction["lanToWan"].(bool)
		a.Direction.LANToLAN, _ = direction["lanToLan"].(bool)
		a.Direction.WANInIDs = stringValues(direction["wanInIds"])
		a.Direction.VPNInIDs = stringValues(direction["vpnInIds"])
	}
	return nil
}

func int64Values(value any) []int64 {
	values, _ := value.([]any)
	result := make([]int64, 0, len(values))
	for _, value := range values {
		result = append(result, int64Value(value))
	}
	return result
}

// ACLConfig contains the ACL fields owned by Terraform.
type ACLConfig struct {
	Type            int64
	Name            string
	Enabled         bool
	Policy          int64
	Protocols       []int64
	SourceType      int64
	SourceIDs       []string
	DestinationType int64
	DestinationIDs  []string
	Direction       ACLDirection
}

func (cfg ACLConfig) fields() map[string]any {
	return map[string]any{
		"type": cfg.Type, "name": cfg.Name, "status": cfg.Enabled, "policy": cfg.Policy,
		"protocols": cfg.Protocols, "sourceType": cfg.SourceType, "sourceIds": cfg.SourceIDs,
		"destinationType": cfg.DestinationType, "destinationIds": cfg.DestinationIDs,
		"direction": map[string]any{"lanToWan": cfg.Direction.LANToWAN, "lanToLan": cfg.Direction.LANToLAN, "wanInIds": cfg.Direction.WANInIDs, "vpnInIds": cfg.Direction.VPNInIDs},
	}
}

func (c *Client) aclsPath(siteID string) string {
	return c.classicPath(fmt.Sprintf("sites/%s/setting/firewall/acls", siteID))
}

// ListACLs returns all rules of one Omada ACL type.
func (c *Client) ListACLs(ctx context.Context, siteID string, aclType int64) ([]ACL, error) {
	return listAllPagesWithQuery[ACL](ctx, c, c.aclsPath(siteID), url.Values{"type": {fmt.Sprint(aclType)}})
}

// FindACL finds an ACL by ID within its type.
func (c *Client) FindACL(ctx context.Context, siteID string, aclType int64, id string) (*ACL, error) {
	rules, err := c.ListACLs(ctx, siteID, aclType)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if rule.ID == id {
			return &rule, nil
		}
	}
	return nil, nil
}

// CreateACL creates a firewall ACL and resolves its generated ID by type/name.
func (c *Client) CreateACL(ctx context.Context, siteID string, cfg ACLConfig) (string, error) {
	fields := cfg.fields()
	fields["customAclPorts"] = []any{}
	fields["customAclDevices"] = []any{}
	fields["stateMode"] = int64(0)
	fields["syslog"] = false
	if err := c.doAuthenticated(ctx, "POST", c.aclsPath(siteID), nil, fields, nil); err != nil {
		return "", err
	}
	rules, err := c.ListACLs(ctx, siteID, cfg.Type)
	if err != nil {
		return "", err
	}
	for _, rule := range rules {
		if rule.Name == cfg.Name {
			return rule.ID, nil
		}
	}
	return "", fmt.Errorf("ACL %q was accepted but was not returned by the controller", cfg.Name)
}

// UpdateACL preserves controller-owned fields while replacing a rule via PUT.
func (c *Client) UpdateACL(ctx context.Context, siteID, id string, cfg ACLConfig) error {
	current, err := c.FindACL(ctx, siteID, cfg.Type, id)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("ACL %q does not exist", id)
	}
	mergeFields(current.Raw, cfg.fields(), "direction")
	current.Raw["id"] = id
	return c.doAuthenticated(ctx, "PUT", fmt.Sprintf("%s/%s", c.aclsPath(siteID), id), nil, current.Raw, nil)
}

// DeleteACL removes a firewall ACL.
func (c *Client) DeleteACL(ctx context.Context, siteID, id string) error {
	return c.doAuthenticated(ctx, "DELETE", fmt.Sprintf("%s/%s", c.aclsPath(siteID), id), nil, nil, nil)
}
