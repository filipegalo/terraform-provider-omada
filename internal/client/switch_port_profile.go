package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// SwitchPortProfile is the VLAN-related portion of an Omada switch port
// profile. Raw is retained because the classic API expects a full object on
// PATCH; preserving it prevents unrelated PoE, STP, LLDP and controller-owned
// settings from being reset when Terraform changes a VLAN association.
type SwitchPortProfile struct {
	ID                 string
	Name               string
	NativeNetworkID    string
	TaggedNetworkIDs   []string
	UntaggedNetworkIDs []string
	VLANConfigEnable   bool
	NetworkTagsSetting int64
	POE                int64
	PortIsolation      bool
	LLDPMed            bool
	Dot1x              int64
	LoopbackDetect     bool
	EEE                bool
	FlowControl        bool
	SpanningTree       bool
	STPPriority        int64
	STPExtPathCost     int64
	STPIntPathCost     int64
	STPP2PLink         int64
	STPEdgePort        bool
	STPLoopProtect     bool
	STPRootProtect     bool
	STPTCGuard         bool
	STPBPDUProtect     bool
	STPBPDUFilter      bool
	STPBPDUForward     bool
	Raw                map[string]any
}

// UnmarshalJSON decodes modelled fields while retaining the complete profile.
func (p *SwitchPortProfile) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.Raw = raw
	p.ID, _ = raw["id"].(string)
	p.Name, _ = raw["name"].(string)
	p.NativeNetworkID, _ = raw["nativeNetworkId"].(string)
	p.TaggedNetworkIDs = stringValues(raw["tagNetworkIds"])
	p.UntaggedNetworkIDs = stringValues(raw["untagNetworkIds"])
	p.VLANConfigEnable, _ = raw["vlanConfigEnable"].(bool)
	p.NetworkTagsSetting = int64Value(raw["networkTagsSetting"])
	p.POE = int64Value(raw["poe"])
	p.PortIsolation, _ = raw["portIsolationEnable"].(bool)
	p.LLDPMed, _ = raw["lldpMedEnable"].(bool)
	p.Dot1x = int64Value(raw["dot1x"])
	p.LoopbackDetect, _ = raw["loopbackDetectEnable"].(bool)
	p.EEE, _ = raw["eeeEnable"].(bool)
	p.FlowControl, _ = raw["flowControlEnable"].(bool)
	p.SpanningTree, _ = raw["spanningTreeEnable"].(bool)
	if stp, ok := raw["spanningTreeSetting"].(map[string]any); ok {
		p.STPPriority = int64Value(stp["priority"])
		p.STPExtPathCost = int64Value(stp["extPathCost"])
		p.STPIntPathCost = int64Value(stp["intPathCost"])
		p.STPP2PLink = int64Value(stp["p2pLink"])
		p.STPEdgePort, _ = stp["edgePort"].(bool)
		p.STPLoopProtect, _ = stp["loopProtect"].(bool)
		p.STPRootProtect, _ = stp["rootProtect"].(bool)
		p.STPTCGuard, _ = stp["tcGuard"].(bool)
		p.STPBPDUProtect, _ = stp["bpduProtect"].(bool)
		p.STPBPDUFilter, _ = stp["bpduFilter"].(bool)
		p.STPBPDUForward, _ = stp["bpduForward"].(bool)
	}
	return nil
}

func stringValues(value any) []string {
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

// SwitchPortProfileConfig contains only the profile fields owned by this
// resource. Pointer fields distinguish an omitted Optional+Computed setting
// from an explicit false/zero/empty value during creation.
type SwitchPortProfileConfig struct {
	Name               string
	NativeNetworkID    *string
	TaggedNetworkIDs   *[]string
	UntaggedNetworkIDs *[]string
	VLANConfigEnable   *bool
	NetworkTagsSetting *int64
	POE                *int64
	PortIsolation      *bool
	LLDPMed            *bool
	Dot1x              *int64
	LoopbackDetect     *bool
	EEE                *bool
	FlowControl        *bool
	SpanningTree       *bool
	STPPriority        *int64
	STPExtPathCost     *int64
	STPIntPathCost     *int64
	STPP2PLink         *int64
	STPEdgePort        *bool
	STPLoopProtect     *bool
	STPRootProtect     *bool
	STPTCGuard         *bool
	STPBPDUProtect     *bool
	STPBPDUFilter      *bool
	STPBPDUForward     *bool
}

func (cfg SwitchPortProfileConfig) fields() map[string]any {
	fields := map[string]any{"name": cfg.Name}
	if cfg.NativeNetworkID != nil {
		fields["nativeNetworkId"] = *cfg.NativeNetworkID
	}
	if cfg.TaggedNetworkIDs != nil {
		fields["tagNetworkIds"] = *cfg.TaggedNetworkIDs
	}
	if cfg.UntaggedNetworkIDs != nil {
		fields["untagNetworkIds"] = *cfg.UntaggedNetworkIDs
	}
	if cfg.VLANConfigEnable != nil {
		fields["vlanConfigEnable"] = *cfg.VLANConfigEnable
	}
	if cfg.NetworkTagsSetting != nil {
		fields["networkTagsSetting"] = *cfg.NetworkTagsSetting
	}
	putPointer(fields, "poe", cfg.POE)
	putPointer(fields, "portIsolationEnable", cfg.PortIsolation)
	putPointer(fields, "lldpMedEnable", cfg.LLDPMed)
	putPointer(fields, "dot1x", cfg.Dot1x)
	putPointer(fields, "loopbackDetectEnable", cfg.LoopbackDetect)
	putPointer(fields, "eeeEnable", cfg.EEE)
	putPointer(fields, "flowControlEnable", cfg.FlowControl)
	putPointer(fields, "spanningTreeEnable", cfg.SpanningTree)
	stp := map[string]any{}
	putPointer(stp, "priority", cfg.STPPriority)
	putPointer(stp, "extPathCost", cfg.STPExtPathCost)
	putPointer(stp, "intPathCost", cfg.STPIntPathCost)
	putPointer(stp, "p2pLink", cfg.STPP2PLink)
	putPointer(stp, "edgePort", cfg.STPEdgePort)
	putPointer(stp, "loopProtect", cfg.STPLoopProtect)
	putPointer(stp, "rootProtect", cfg.STPRootProtect)
	putPointer(stp, "tcGuard", cfg.STPTCGuard)
	putPointer(stp, "bpduProtect", cfg.STPBPDUProtect)
	putPointer(stp, "bpduFilter", cfg.STPBPDUFilter)
	putPointer(stp, "bpduForward", cfg.STPBPDUForward)
	if len(stp) > 0 {
		fields["spanningTreeSetting"] = stp
	}
	return fields
}

func putPointer[T any](target map[string]any, key string, value *T) {
	if value != nil {
		target[key] = *value
	}
}

func (c *Client) switchPortProfilesPath(siteID string) string {
	return c.classicPath(fmt.Sprintf("sites/%s/setting/lan/profiles", siteID))
}

// ListSwitchPortProfiles returns every reusable switch port profile on a site.
func (c *Client) ListSwitchPortProfiles(ctx context.Context, siteID string) ([]SwitchPortProfile, error) {
	return listAllPages[SwitchPortProfile](ctx, c, c.switchPortProfilesPath(siteID))
}

// FindSwitchPortProfile finds a profile by its controller ID.
func (c *Client) FindSwitchPortProfile(ctx context.Context, siteID, id string) (*SwitchPortProfile, error) {
	profiles, err := c.ListSwitchPortProfiles(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return &profile, nil
		}
	}
	return nil, nil
}

// CreateSwitchPortProfile creates a profile and returns its controller ID.
func (c *Client) CreateSwitchPortProfile(ctx context.Context, siteID string, cfg SwitchPortProfileConfig) (string, error) {
	if err := c.doAuthenticated(ctx, "POST", c.switchPortProfilesPath(siteID), nil, cfg.fields(), nil); err != nil {
		return "", err
	}
	profiles, err := c.ListSwitchPortProfiles(ctx, siteID)
	if err != nil {
		return "", fmt.Errorf("listing switch port profiles after creation: %w", err)
	}
	for _, profile := range profiles {
		if profile.Name == cfg.Name {
			return profile.ID, nil
		}
	}
	return "", fmt.Errorf("switch port profile %q was accepted but was not returned by the controller", cfg.Name)
}

// UpdateSwitchPortProfile updates owned fields while preserving the full object.
func (c *Client) UpdateSwitchPortProfile(ctx context.Context, siteID, id string, cfg SwitchPortProfileConfig) error {
	current, err := c.FindSwitchPortProfile(ctx, siteID, id)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("switch port profile %q does not exist", id)
	}
	// The endpoint requires a full object. Overlay only Terraform-owned fields
	// onto the freshly read document and leave every other key untouched.
	mergeFields(current.Raw, cfg.fields(), "spanningTreeSetting")
	return c.doAuthenticated(ctx, "PATCH", fmt.Sprintf("%s/%s", c.switchPortProfilesPath(siteID), id), nil, current.Raw, nil)
}

// DeleteSwitchPortProfile removes a profile from a site.
func (c *Client) DeleteSwitchPortProfile(ctx context.Context, siteID, id string) error {
	return c.doAuthenticated(ctx, "DELETE", fmt.Sprintf("%s/%s", c.switchPortProfilesPath(siteID), id), nil, nil, nil)
}
