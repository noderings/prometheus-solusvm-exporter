package svmclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	pageSize   int
}

func New(baseURL, token string, insecureSkipTLS bool) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		pageSize: 100,
	}
}

func (c *Client) apiURL(path string) string {
	return c.baseURL + "/api/v1" + path
}

type PaginatedResponse struct {
	Data json.RawMessage `json:"data"`
	Meta struct {
		LastPage   int `json:"last_page"`
		Pagination struct {
			CurrentPage int `json:"current_page"`
			TotalPages  int `json:"total_pages"`
			LastPage    int `json:"last_page"`
		} `json:"pagination"`
	} `json:"meta"`
}

func (pr PaginatedResponse) lastPage() int {
	if pr.Meta.Pagination.TotalPages > 0 {
		return pr.Meta.Pagination.TotalPages
	}
	if pr.Meta.Pagination.LastPage > 0 {
		return pr.Meta.Pagination.LastPage
	}
	return pr.Meta.LastPage
}

func (c *Client) paginate(ctx context.Context, path string, extra url.Values) ([]json.RawMessage, error) {
	var all []json.RawMessage
	page := 1
	for {
		q := url.Values{}
		for k, vs := range extra {
			q[k] = vs
		}
		q.Set("page", strconv.Itoa(page))
		q.Set("per_page", strconv.Itoa(c.pageSize))
		body, err := c.do(ctx, http.MethodGet, path, q)
		if err != nil {
			return nil, err
		}
		var pr PaginatedResponse
		if err := json.Unmarshal(body, &pr); err != nil {
			return nil, fmt.Errorf("decode page %d of %s: %w", page, path, err)
		}
		var items []json.RawMessage
		if len(pr.Data) > 0 && string(pr.Data) != "null" {
			if err := json.Unmarshal(pr.Data, &items); err != nil {
				return nil, fmt.Errorf("decode data page %d of %s: %w", page, path, err)
			}
		}
		all = append(all, items...)
		last := pr.lastPage()
		if last <= 0 || page >= last {
			break
		}
		page++
	}
	return all, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values) ([]byte, error) {
	u := c.apiURL(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d from %s %s: %s", resp.StatusCode, method, path, truncate(body, 256))
	}
	return body, nil
}

func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}

// GetAccount checks API connectivity: GET /account.
func (c *Client) GetAccount(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/account", nil)
	return err
}

// ListServers returns all servers (paginated).
func (c *Client) ListServers(ctx context.Context) ([]json.RawMessage, error) {
	return c.paginate(ctx, "/servers", nil)
}

type LimitPool struct {
	Total     float64 `json:"total"`
	Used      float64 `json:"used"`
	Unlimited bool    `json:"unlimited"`
}

type ResourceLimits struct {
	VCPU LimitPool `json:"vcpu"`
	RAM  LimitPool `json:"ram"`
	HDD  LimitPool `json:"hdd"`
}

type ComputeResource struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	IsLocked   bool   `json:"is_locked"`
	LocationID int    `json:"location_id"`
	Location   *struct {
		ID int `json:"id"`
	} `json:"location"`
	Locations []struct {
		ID int `json:"id"`
	} `json:"locations"`
	Limits   ResourceLimits `json:"limits"`
	Settings struct {
		Limits ResourceLimits `json:"limits"`
	} `json:"settings"`
	Metrics struct {
		Memory struct {
			Total float64 `json:"total"`
		} `json:"memory"`
	} `json:"metrics"`
}

func (h ComputeResource) ResolvedLocationID() int {
	if h.LocationID != 0 {
		return h.LocationID
	}
	if h.Location != nil && h.Location.ID != 0 {
		return h.Location.ID
	}
	if len(h.Locations) > 0 {
		return h.Locations[0].ID
	}
	return 0
}

func (h ComputeResource) ResolvedLimits() ResourceLimits {
	if h.Settings.Limits.VCPU.Total != 0 || h.Settings.Limits.RAM.Total != 0 || h.Settings.Limits.HDD.Total != 0 {
		return h.Settings.Limits
	}
	return h.Limits
}

func (h ComputeResource) IsUp() bool {
	if h.IsLocked {
		return false
	}
	st := strings.ToLower(strings.TrimSpace(h.Status))
	return st == "active" || st == "online" || st == ""
}

// ListComputeResources returns GET /compute_resources (paginated).
func (c *Client) ListComputeResources(ctx context.Context) ([]ComputeResource, error) {
	raw, err := c.paginate(ctx, "/compute_resources", nil)
	if err != nil {
		return nil, err
	}
	out := make([]ComputeResource, 0, len(raw))
	for _, item := range raw {
		var h ComputeResource
		if err := json.Unmarshal(item, &h); err != nil {
			return nil, fmt.Errorf("decode compute resource: %w", err)
		}
		out = append(out, h)
	}
	return out, nil
}

// GetUsage returns GET /usage/{kind}/{uuid} (cpu|memory|network|disks).
func (c *Client) GetUsage(ctx context.Context, kind, uuid string) (json.RawMessage, error) {
	path := fmt.Sprintf("/usage/%s/%s", kind, uuid)
	body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// GetServer returns GET /servers/{id} (used for usage.cpu fallback).
func (c *Client) GetServer(ctx context.Context, serverID int) (json.RawMessage, error) {
	path := fmt.Sprintf("/servers/%d", serverID)
	body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}
