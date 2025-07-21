package status

import (
	"net/url"
	"time"
)

type TypeMeta struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
}

type Environment struct {
	TypeMeta `json:",inline"`
	Name     string `json:"name"`
}

type EnvironmentList struct {
	TypeMeta `json:",inline"`
	Items    []Environment `json:"items"`
}

type Release struct {
	TypeMeta `json:",inline"`
	Name     string `json:"name"`
	SHA      string `json:"sha"`
}

type ReleaseList struct {
	TypeMeta `json:",inline"`
	Items    []Release `json:"items"`
}

type EnvironmentRelease struct {
	TypeMeta    `json:",inline"`
	Name        string                        `json:"name"`
	ReleaseName string                        `json:"releaseName"`
	SHA         string                        `json:"sha"`
	Environment string                        `json:"environment"`
	Images      map[string]*DeployedImageInfo `json:"images"`
}

type EnvironmentReleaseList struct {
	TypeMeta `json:",inline"`
	Items    []EnvironmentRelease `json:"items"`
}

type DeployedImageInfo struct {
	Name                 string `json:"name"`
	ImageInfo            ContainerImage
	ImageCreationTime    *time.Time `json:"imageCreationTime,omitempty"`
	RepoLink             *url.URL   `json:"repoLink"`
	SourceSHA            string     `json:"sourceSHA"`
	PermLinkForSourceSHA *url.URL   `json:"permLinkForSourceSHA,omitempty"`
}

type ContainerImage struct {
	Digest     string `json:"digest"`
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
}
