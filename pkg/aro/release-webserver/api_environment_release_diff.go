package release_webserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetEnvironmentReleaseDiff(accessor ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		environmentReleaseName := c.Param("name")
		environmentName, releaseName, found := SplitEnvironmentReleaseName(environmentReleaseName)
		if !found {
			c.String(http.StatusBadRequest, "failed to parse environment release name: %q (expected format <environmentName>---<releaseName>", environmentReleaseName)
			return
		}
		otherEnvironmentReleaseName := c.Param("otherName")
		otherEnvironmentName, otherReleaseName, found := SplitEnvironmentReleaseName(otherEnvironmentReleaseName)
		if !found {
			c.String(http.StatusBadRequest, "failed to parse environment release name: %q (expected format <environmentName>---<releaseName>", otherEnvironmentReleaseName)
			return
		}

		releases, err := accessor.ListReleases(ctx)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed to list releases: %v", err)
			return
		}

		var release *Release
		var otherRelease *Release
		for _, currRelease := range releases {
			if currRelease.Name == releaseName {
				release = &currRelease
				break
			}
		}
		for _, currRelease := range releases {
			if currRelease.Name == otherReleaseName {
				otherRelease = &currRelease
				break
			}
		}
		if release == nil || otherRelease == nil {
			c.String(http.StatusInternalServerError, "failed to find release %q or %q: %v", releaseName, otherReleaseName, err)
			return
		}

		ret, err := accessor.GetReleaseEnvironmentDiff(ctx, *release, environmentName, *otherRelease, otherEnvironmentName)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed to get release environment diff for name=%q to other=%q: %v", environmentReleaseName, otherEnvironmentReleaseName, err)
			return
		}

		c.IndentedJSON(http.StatusOK, ret)
	}
}
