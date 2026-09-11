package client

import (
	"context"
	"fmt"
	"strings"
)

// WLANGroup groups SSIDs that are deployed together by Omada.
type WLANGroup struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Primary bool   `json:"primary"`
}

func (c *Client) wlanGroupsPath(siteID string) string {
	return c.classicPath(fmt.Sprintf("sites/%s/setting/wlans", siteID))
}

// ListWLANGroups returns all WLAN groups on a site. Some controller builds
// return a paginated object and others return a bare array, so both are handled.
func (c *Client) ListWLANGroups(ctx context.Context, siteID string) ([]WLANGroup, error) {
	path := c.wlanGroupsPath(siteID)
	groups, err := listAllPages[WLANGroup](ctx, c, path)
	if err != nil {
		var bare []WLANGroup
		if bareErr := c.doAuthenticated(ctx, "GET", path, nil, nil, &bare); bareErr == nil {
			return bare, nil
		}
		return nil, err
	}
	if len(groups) != 0 {
		return groups, nil
	}
	var bare []WLANGroup
	if err := c.doAuthenticated(ctx, "GET", path, nil, nil, &bare); err != nil {
		return nil, err
	}
	return bare, nil
}

// FindWLANGroup finds a WLAN group by controller ID.
func (c *Client) FindWLANGroup(ctx context.Context, siteID, id string) (*WLANGroup, error) {
	groups, err := c.ListWLANGroups(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.ID == id {
			return &group, nil
		}
	}
	return nil, nil
}

// FindWLANGroupByName finds a uniquely named WLAN group case-insensitively.
func (c *Client) FindWLANGroupByName(ctx context.Context, siteID, name string) (*WLANGroup, error) {
	groups, err := c.ListWLANGroups(ctx, siteID)
	if err != nil {
		return nil, err
	}
	var found *WLANGroup
	for i := range groups {
		if strings.EqualFold(groups[i].Name, name) {
			if found != nil {
				return nil, fmt.Errorf("multiple WLAN groups are named %q", name)
			}
			candidate := groups[i]
			found = &candidate
		}
	}
	return found, nil
}

// CreateWLANGroup creates a group and resolves the controller-generated ID.
func (c *Client) CreateWLANGroup(ctx context.Context, siteID, name string) (string, error) {
	body := map[string]any{"name": name, "clone": false}
	if err := c.doAuthenticated(ctx, "POST", c.wlanGroupsPath(siteID), nil, body, nil); err != nil {
		return "", err
	}
	group, err := c.FindWLANGroupByName(ctx, siteID, name)
	if err != nil {
		return "", err
	}
	if group == nil {
		return "", fmt.Errorf("WLAN group %q was accepted but was not returned by the controller", name)
	}
	return group.ID, nil
}

// UpdateWLANGroup renames a WLAN group.
func (c *Client) UpdateWLANGroup(ctx context.Context, siteID, id, name string) error {
	body := map[string]any{"name": name, "clone": false}
	return c.doAuthenticated(ctx, "PATCH", fmt.Sprintf("%s/%s", c.wlanGroupsPath(siteID), id), nil, body, nil)
}

// DeleteWLANGroup removes a WLAN group.
func (c *Client) DeleteWLANGroup(ctx context.Context, siteID, id string) error {
	return c.doAuthenticated(ctx, "DELETE", fmt.Sprintf("%s/%s", c.wlanGroupsPath(siteID), id), nil, nil, nil)
}
