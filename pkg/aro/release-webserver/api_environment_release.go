package release_webserver

import (
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
				currReleaseEnvironmentInfo, err := accessor.GetReleaseEnvironmentInfo(ctx, release, environment)
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

		name := c.Param("name")
		environmentName, releaseName, found := SplitEnvironmentReleaseName(name)
		if !found {
			c.IndentedJSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("%q must be in format <environmentName>---<releaseName>", name)})
			return
		}

		// TODO better
		releases, err := accessor.ListReleases(ctx)
		if err != nil {
			c.String(500, "failed to list environments: %v", err)
			return
		}

		var release *Release
		for _, currRelease := range releases {
			if currRelease.Name == releaseName {
				release = &currRelease
				break
			}
		}
		if release == nil {
			c.String(500, "failed to find release %q: %v", releaseName, err)
			return
		}

		currReleaseEnvironmentInfo, _ := accessor.GetReleaseEnvironmentInfo(ctx, *release, environmentName)
		if currReleaseEnvironmentInfo == nil {
			c.IndentedJSON(http.StatusNotFound, gin.H{"message": fmt.Sprintf("%q not found", name)})
			return
		}

		ret := accessorEnvInfoToReleaseInfo(currReleaseEnvironmentInfo)

		c.IndentedJSON(http.StatusOK, ret)
	}
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
		Images:      map[string]*status.ComponentInfo{},
	}
	for _, imageInfo := range currReleaseEnvironmentInfo.DeployedImages {
		ret.Images[imageInfo.Name] = &status.ComponentInfo{
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
			ret.Images[imageInfo.Name].RepoURL = ptr.To(imageInfo.RepoLink.String())
		}
		if imageInfo.PermLinkForSourceSHA != nil {
			ret.Images[imageInfo.Name].PermanentURLForSourceSHA = ptr.To(imageInfo.PermLinkForSourceSHA.String())
		}
		if imageInfo.ImageInfo != nil {
			ret.Images[imageInfo.Name].ImageInfo.Digest = imageInfo.ImageInfo.Digest
			ret.Images[imageInfo.Name].ImageInfo.Registry = ptr.Deref(imageInfo.ImageInfo.Registry, "MISSING REGISTRY")
			ret.Images[imageInfo.Name].ImageInfo.Repository = imageInfo.ImageInfo.Repository
		}
	}
	return ret
}
