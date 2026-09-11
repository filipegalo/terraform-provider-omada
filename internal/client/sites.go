package client

import (
	"context"
	"fmt"
	"strings"
)

// Site is a controller site as returned by the sites list endpoint.
type Site struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListSites returns every site visible to the configured controller account.
func (c *Client) ListSites(ctx context.Context) ([]Site, error) {
	return listAllPages[Site](ctx, c, c.classicPath("sites"))
}

// ResolveSite resolves a site name or ID to its ID. An exact ID match wins;
// otherwise sites are matched by name, case-insensitively.
func (c *Client) ResolveSite(ctx context.Context, nameOrID string) (string, error) {
	sites, err := c.ListSites(ctx)
	if err != nil {
		return "", fmt.Errorf("listing sites: %w", err)
	}

	for _, s := range sites {
		if s.ID == nameOrID {
			return s.ID, nil
		}
	}
	for _, s := range sites {
		if strings.EqualFold(s.Name, nameOrID) {
			return s.ID, nil
		}
	}

	return "", fmt.Errorf("no site found with name or ID %q", nameOrID)
}
