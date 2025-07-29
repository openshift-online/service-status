package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/openshift-online/service-status/pkg/apis/status"
)

type fileBasedReleaseClient struct {
	fs fs.FS
}

func NewFileSystemReleaseClient(fs fs.FS) ReleaseClient {
	return &fileBasedReleaseClient{
		fs: fs,
	}
}

func (c *fileBasedReleaseClient) get(ctx context.Context, path string) ([]byte, error) {
	return fs.ReadFile(c.fs, path)
}

func (c *fileBasedReleaseClient) ListEnvironments(ctx context.Context) (*status.EnvironmentList, error) {
	url := filepath.Join("api/aro-hcp/environments.json")
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

func (c *fileBasedReleaseClient) GetEnvironment(ctx context.Context, name string) (*status.Environment, error) {
	url := filepath.Join("api/aro-hcp/environments.json", name)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var list status.EnvironmentList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	for _, item := range list.Items {
		if item.Name == name {
			return &item, nil
		}
	}

	return nil, fmt.Errorf("environment %v not found", name)
}

func (c *fileBasedReleaseClient) ListReleases(ctx context.Context) (*status.ReleaseList, error) {
	url := filepath.Join("api/aro-hcp/releases.json")
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

func (c *fileBasedReleaseClient) GetRelease(ctx context.Context, name string) (*status.Release, error) {
	url := filepath.Join("api/aro-hcp/releases.json")
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var list status.ReleaseList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	for _, item := range list.Items {
		if item.Name == name {
			return &item, nil
		}
	}

	return nil, fmt.Errorf("release %v not found", name)
}

func (c *fileBasedReleaseClient) ListEnvironmentReleases(ctx context.Context) (*status.EnvironmentReleaseList, error) {
	url := filepath.Join("api/aro-hcp/environmentreleases.json")
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

func (c *fileBasedReleaseClient) GetEnvironmentRelease(ctx context.Context, environmentName, releaseName string) (*status.EnvironmentRelease, error) {
	url := filepath.Join("api/aro-hcp/environmentreleases.json")
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var list status.EnvironmentReleaseList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	for _, item := range list.Items {
		if item.ReleaseName == releaseName && item.Environment == environmentName {
			return &item, nil
		}
	}

	return nil, fmt.Errorf("environmentrelease %v not found", fmt.Sprintf("%v---%v", environmentName, releaseName))
}
