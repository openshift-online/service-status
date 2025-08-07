package release_webserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetEnvironmentReleaseDiff(accessor ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

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
