package release_webserver

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/apis/status"
)

func ListReleases(accessor ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		releases, err := accessor.ListReleases(ctx)
		if err != nil {
			c.String(500, "failed to list releases: %v", err)
			return
		}

		ret := status.ReleaseList{
			TypeMeta: status.TypeMeta{
				Kind:       "ReleaseList",
				APIVersion: "service-status.hcm.openshift.io/v1",
			},
			Items: []status.Release{},
		}
		for _, release := range releases {
			ret.Items = append(ret.Items, status.Release{
				TypeMeta: status.TypeMeta{
					Kind:       "Release",
					APIVersion: "service-status.hcm.openshift.io/v1",
				},
				Name: release.Name,
				SHA:  release.Commit.Hash.String(),
			})
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
		for _, release := range releases {
			if release.Name == name {
				ret := status.Release{
					TypeMeta: status.TypeMeta{
						Kind:       "Release",
						APIVersion: "service-status.hcm.openshift.io/v1",
					},
					Name: release.Name,
					SHA:  release.Commit.Hash.String(),
				}
				c.IndentedJSON(http.StatusOK, ret)
			}
		}

		c.IndentedJSON(http.StatusNotFound, gin.H{"message": fmt.Sprintf("%q not found", name)})
	}
}
