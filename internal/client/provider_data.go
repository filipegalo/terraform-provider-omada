package client

// ProviderData is what the provider's Configure stores for resources to
// retrieve via req.ProviderData.
type ProviderData struct {
	Client *Client
	SiteID string
}
