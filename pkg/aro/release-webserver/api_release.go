package release_webserver

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func ListReleases(accessor ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		ret, err := accessor.ListReleases(ctx)
		if err != nil {
			c.String(500, "failed to list releases: %v", err)
			return
		}

		c.IndentedJSON(http.StatusOK, ret)
	}
}

func GetRelease(accessor ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		releases, err := accessor.ListReleases(ctx)
		if err != nil {
			c.String(500, "failed to list environments: %v", err)
			return
		}

		name := c.Param("name")
		for _, release := range releases.Items {
			if release.Name == name {
				c.IndentedJSON(http.StatusOK, release)
				return
			}
		}

		c.IndentedJSON(http.StatusNotFound, gin.H{"message": fmt.Sprintf("%q not found", name)})
	}
}
