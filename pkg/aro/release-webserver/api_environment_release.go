package release_webserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/apis/status"
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
			for _, release := range releases.Items {
				currReleaseEnvironmentInfo, err := accessor.GetReleaseEnvironmentInfo(ctx, GetEnvironmentReleaseName(environment, release.Name))
				if err != nil {
					c.String(500, "failed to get release env env=%q, release=%q: %v", environment, release, err)
					return
				}
				if currReleaseEnvironmentInfo == nil {
					continue
				}
				ret.Items = append(ret.Items, *currReleaseEnvironmentInfo)
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
	currReleaseEnvironmentInfo, err := accessor.GetReleaseEnvironmentInfo(ctx, environmentReleaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release environment info: %w", err)
	}
	if currReleaseEnvironmentInfo == nil {
		return nil, fmt.Errorf("%q not found", environmentReleaseName)
	}

	return currReleaseEnvironmentInfo, nil
}
