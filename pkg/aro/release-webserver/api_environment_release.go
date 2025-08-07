package release_webserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/apis/status"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"k8s.io/utils/ptr"
)

func ListEnvironmentReleases(accessor ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		environments, err := accessor.ListEnvironments(ctx)
		if err != nil {
			c.String(500, "failed to list environments: %v", err)
			return
		}

		releases, err := accessor.ListReleases(ctx)
		if err != nil {
			c.String(500, "failed to list releases: %v", err)
			return
		}

		ret := status.EnvironmentReleaseList{
			TypeMeta: status.TypeMeta{
				Kind:       "EnvironmentReleaseList",
				APIVersion: "service-status.hcm.openshift.io/v1",
			},
			Items: []status.EnvironmentRelease{},
		}
		for _, environment := range environments {
			for _, release := range releases {
				currReleaseEnvironmentInfo, err := accessor.GetReleaseEnvironmentInfo(ctx, GetEnvironmentReleaseName(environment, release.Name))
				if err != nil {
					c.String(500, "failed to get release env env=%q, release=%q: %v", environment, release, err)
					return
				}
				if currReleaseEnvironmentInfo == nil {
					continue
				}
				ret.Items = append(ret.Items, *accessorEnvInfoToReleaseInfo(currReleaseEnvironmentInfo))

			}
		}

		c.IndentedJSON(http.StatusOK, ret)
	}
}

func GetEnvironmentReleaseName(environment, release string) string {
	return fmt.Sprintf("%s---%s", environment, release)
}

func SplitEnvironmentReleaseName(name string) (string, string, bool) {
	parts := strings.Split(name, "---")
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func GetEnvironmentRelease(accessor ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		environmentReleaseName := c.Param("name")
		ret, err := getEnvironmentRelease(ctx, accessor, environmentReleaseName)
		if err != nil && strings.Contains(err.Error(), "not found") {
			c.String(http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			c.String(http.StatusInternalServerError, "failed to get release info: %v", err)
			return
		}

		c.IndentedJSON(http.StatusOK, ret)
	}
}

func getEnvironmentRelease(ctx context.Context, accessor ReleaseAccessor, environmentReleaseName string) (*status.EnvironmentRelease, error) {
	currReleaseEnvironmentInfo, _ := accessor.GetReleaseEnvironmentInfo(ctx, environmentReleaseName)
	if currReleaseEnvironmentInfo == nil {
		return nil, fmt.Errorf("%q not found", environmentReleaseName)
	}

	ret := accessorEnvInfoToReleaseInfo(currReleaseEnvironmentInfo)
	return ret, nil
}

func accessorEnvInfoToReleaseInfo(currReleaseEnvironmentInfo *release_inspection.ReleaseEnvironmentInfo) *status.EnvironmentRelease {
	if currReleaseEnvironmentInfo == nil {
		return nil
	}
	ret := &status.EnvironmentRelease{
		TypeMeta: status.TypeMeta{
			Kind:       "EnvironmentRelease",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Name:        fmt.Sprintf("%s---%s", currReleaseEnvironmentInfo.EnvironmentFilename, currReleaseEnvironmentInfo.ReleaseName),
		ReleaseName: currReleaseEnvironmentInfo.ReleaseName,
		SHA:         currReleaseEnvironmentInfo.ReleaseSHA,
		Environment: currReleaseEnvironmentInfo.EnvironmentFilename,
		Components:  map[string]*status.ComponentInfo{},
	}
	for _, imageInfo := range currReleaseEnvironmentInfo.Components {
		ret.Components[imageInfo.Name] = &status.ComponentInfo{
			Name: imageInfo.Name,
			ImageInfo: status.ContainerImage{
				Digest:     "",
				Registry:   "",
				Repository: "",
			},
			ImageCreationTime: imageInfo.ImageCreationTime,
			SourceSHA:         imageInfo.SourceSHA,
		}
		if imageInfo.RepoLink != nil {
			ret.Components[imageInfo.Name].RepoURL = ptr.To(imageInfo.RepoLink.String())
		}
		if imageInfo.PermLinkForSourceSHA != nil {
			ret.Components[imageInfo.Name].PermanentURLForSourceSHA = ptr.To(imageInfo.PermLinkForSourceSHA.String())
		}
		if imageInfo.ImageInfo != nil {
			ret.Components[imageInfo.Name].ImageInfo.Digest = imageInfo.ImageInfo.Digest
			ret.Components[imageInfo.Name].ImageInfo.Registry = ptr.Deref(imageInfo.ImageInfo.Registry, "MISSING REGISTRY")
			ret.Components[imageInfo.Name].ImageInfo.Repository = imageInfo.ImageInfo.Repository
		}
	}
	return ret
}
