package release_webserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/apis/status"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"k8s.io/klog/v2"
)

func ListEnvironmentReleases(accessor release_inspection.ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		logger := klog.LoggerWithValues(klog.FromContext(ctx), "URL", c.Request.URL)
		ctx = klog.NewContext(ctx, logger)

		ret, err := accessor.ListEnvironmentReleases(ctx)
		if err != nil {
			c.String(500, "failed to list releases: %v", err)
			return
		}

		c.IndentedJSON(http.StatusOK, ret)
	}
}

func ListEnvironmentReleasesForEnvironment(accessor release_inspection.ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		logger := klog.LoggerWithValues(klog.FromContext(ctx), "URL", c.Request.URL)
		ctx = klog.NewContext(ctx, logger)

		environmentName := c.Param("name")

		environmentReleases, err := accessor.ListEnvironmentReleasesForEnvironment(ctx, environmentName)
		if err != nil {
			c.String(500, "failed to list releases: %v", err)
			return
		}

		c.IndentedJSON(http.StatusOK, environmentReleases)
	}
}

func GetEnvironmentRelease(accessor release_inspection.ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		logger := klog.LoggerWithValues(klog.FromContext(ctx), "URL", c.Request.URL)
		ctx = klog.NewContext(ctx, logger)

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

func getEnvironmentRelease(ctx context.Context, accessor release_inspection.ReleaseAccessor, environmentReleaseName string) (*status.EnvironmentRelease, error) {
	currReleaseEnvironmentInfo, err := accessor.GetEnvironmentRelease(ctx, environmentReleaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release environment info: %w", err)
	}
	if currReleaseEnvironmentInfo == nil {
		return nil, fmt.Errorf("%q not found", environmentReleaseName)
	}

	return currReleaseEnvironmentInfo, nil
}
