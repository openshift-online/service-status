package release_webserver

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/apis/status"
)

func ListEnvironments(accessor ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		environments, err := accessor.ListEnvironments(ctx)
		if err != nil {
			c.String(500, "failed to list environments: %v", err)
			return
		}

		ret := status.EnvironmentList{
			TypeMeta: status.TypeMeta{
				Kind:       "EnvironmentList",
				APIVersion: "service-status.hcm.openshift.io/v1",
			},
			Items: []status.Environment{},
		}
		for _, environment := range environments {
			ret.Items = append(ret.Items, status.Environment{
				TypeMeta: status.TypeMeta{
					Kind:       "Environment",
					APIVersion: "service-status.hcm.openshift.io/v1",
				},
				Name: environment,
			})
		}

		c.IndentedJSON(http.StatusOK, ret)
	}
}

func GetEnvironment(accessor ReleaseAccessor) func(c *gin.Context) {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		environments, err := accessor.ListEnvironments(ctx)
		if err != nil {
			c.String(500, "failed to list environments: %v", err)
			return
		}

		name := c.Param("name")
		for _, environment := range environments {
			if environment == name {
				ret := status.Environment{
					TypeMeta: status.TypeMeta{
						Kind:       "Environment",
						APIVersion: "service-status.hcm.openshift.io/v1",
					},
					Name: environment,
				}
				c.IndentedJSON(http.StatusOK, ret)
			}
		}

		c.IndentedJSON(http.StatusNotFound, gin.H{"message": fmt.Sprintf("%q not found", name)})
	}
}
