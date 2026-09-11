package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

const (
	pageSize = 100

	// maxPages bounds listAllPages against a controller that reports a
	// "not done yet" totalRows/currentPage/currentSize combination on every
	// page (as happened live: an unrecognized pagination parameter made the
	// controller return an empty page with a stale currentPage/currentSize
	// forever). 1000 pages is far beyond any real reservation or site
	// count; hitting it means the loop's completion check is being fooled
	// again, and that should surface as an error, not a hang.
	maxPages = 1000
)

// pageResult is the paginated list envelope shared by every list endpoint.
type pageResult[T any] struct {
	CurrentPage int `json:"currentPage"`
	CurrentSize int `json:"currentSize"`
	TotalRows   int `json:"totalRows"`
	Data        []T `json:"data"`
}

// paginationParams builds the classic API's pagination query parameters.
// Every endpoint this client calls (sites, DHCP reservations) is a classic
// endpoint, and confirmed live against a 6.3.0.45 controller, they only
// recognize currentPage/currentPageSize -- a version-gated page/pageSize
// variant was tried here previously and is wrong: the controller silently
// answers with an empty page and an unchanged currentPage/currentSize,
// which never satisfies the "done" check below and loops forever.
func paginationParams(page int) url.Values {
	q := url.Values{}
	q.Set("currentPage", strconv.Itoa(page))
	q.Set("currentPageSize", strconv.Itoa(pageSize))
	return q
}

// listAllPages drains a paginated authenticated GET endpoint, following
// pages until the controller reports no rows remain.
func listAllPages[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	return listAllPagesWithQuery[T](ctx, c, path, nil)
}

// listAllPagesWithQuery drains a list endpoint while retaining endpoint-specific
// filters such as ACL type on every page.
func listAllPagesWithQuery[T any](ctx context.Context, c *Client, path string, baseQuery url.Values) ([]T, error) {
	var all []T
	page := 1
	for {
		var res pageResult[T]
		query := paginationParams(page)
		for key, values := range baseQuery {
			query[key] = append([]string(nil), values...)
		}
		if err := c.doAuthenticated(ctx, "GET", path, query, nil, &res); err != nil {
			return nil, err
		}
		all = append(all, res.Data...)
		if res.TotalRows <= res.CurrentPage*res.CurrentSize {
			break
		}
		page++
		if page > maxPages {
			return nil, fmt.Errorf("paginating %s: exceeded %d pages without the controller reporting completion (got totalRows=%d currentPage=%d currentSize=%d) -- the controller's response no longer matches what this client expects", path, maxPages, res.TotalRows, res.CurrentPage, res.CurrentSize)
		}
	}
	return all, nil
}
