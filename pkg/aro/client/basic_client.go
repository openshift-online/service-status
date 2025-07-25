package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/openshift-online/service-status/pkg/apis/status"
)

type ReleaseClient interface {
	ListEnvironments(ctx context.Context) (*status.EnvironmentList, error)
	GetEnvironment(ctx context.Context, name string) (*status.Environment, error)
	ListReleases(ctx context.Context) (*status.ReleaseList, error)
	GetRelease(ctx context.Context, name string) (*status.Release, error)
	ListEnvironmentReleases(ctx context.Context) (*status.EnvironmentReleaseList, error)
	GetEnvironmentRelease(ctx context.Context, environmentName, releaseName string) (*status.EnvironmentRelease, error)
}

type basicReleaseClient struct {
	baseURL string
	client  *http.Client
}

func NewBasicReleaseClient(baseURL string) ReleaseClient {
	return &basicReleaseClient{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (c *basicReleaseClient) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode > 299 || resp.StatusCode < 200 {
		return nil, fmt.Errorf("request failed: %v: %v", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *basicReleaseClient) ListEnvironments(ctx context.Context) (*status.EnvironmentList, error) {
	url := fmt.Sprintf("%s/api/aro-hcp/environments", c.baseURL)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}

	var result status.EnvironmentList
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *basicReleaseClient) GetEnvironment(ctx context.Context, name string) (*status.Environment, error) {
	url := fmt.Sprintf("%s/api/aro-hcp/environments/%v", c.baseURL, url.PathEscape(name))
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}

	var result status.Environment
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *basicReleaseClient) ListReleases(ctx context.Context) (*status.ReleaseList, error) {
	url := fmt.Sprintf("%s/api/aro-hcp/releases", c.baseURL)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}

	var result status.ReleaseList
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *basicReleaseClient) GetRelease(ctx context.Context, name string) (*status.Release, error) {
	url := fmt.Sprintf("%s/api/aro-hcp/releases/%v", c.baseURL, url.PathEscape(name))
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}

	var result status.Release
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *basicReleaseClient) ListEnvironmentReleases(ctx context.Context) (*status.EnvironmentReleaseList, error) {
	url := fmt.Sprintf("%s/api/aro-hcp/environmentreleases", c.baseURL)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}

	var result status.EnvironmentReleaseList
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *basicReleaseClient) GetEnvironmentRelease(ctx context.Context, environmentName, releaseName string) (*status.EnvironmentRelease, error) {
	url := fmt.Sprintf("%s/api/aro-hcp/environmentreleases/%v---%v", c.baseURL, url.PathEscape(environmentName), url.PathEscape(releaseName))
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}

	var result status.EnvironmentRelease
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
