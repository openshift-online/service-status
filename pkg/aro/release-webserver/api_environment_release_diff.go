package release_webserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"k8s.io/klog/v2"
)

func GetEnvironmentReleaseDiff(accessor release_inspection.ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		logger := klog.LoggerWithValues(klog.FromContext(ctx), "URL", c.Request.URL)
		ctx = klog.NewContext(ctx, logger)

		environmentReleaseName := c.Param("name")
		otherEnvironmentReleaseName := c.Param("otherName")

		ret, err := accessor.GetReleaseEnvironmentDiff(ctx, environmentReleaseName, otherEnvironmentReleaseName)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed to get release environment diff for name=%q to other=%q: %v", environmentReleaseName, otherEnvironmentReleaseName, err)
			return
		}

		c.IndentedJSON(http.StatusOK, ret)
	}
}
