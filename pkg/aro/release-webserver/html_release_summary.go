package release_webserver

import (
	"fmt"
	"html/template"
	"net/url"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/apis/status"
	"github.com/openshift-online/service-status/pkg/aro/client"
	"k8s.io/utils/set"
)

type htmlReleaseSummary struct {
	releaseClient client.ReleaseClient
}

func (h *htmlReleaseSummary) ServeGin(c *gin.Context) {
	ctx := c.Request.Context()

	environments, err := h.releaseClient.ListEnvironments(ctx)
	if err != nil {
		c.String(500, "failed to list environments: %v", err)
		return
	}

	releases, err := h.releaseClient.ListReleases(ctx)
	if err != nil {
		c.String(500, "failed to list releases: %v", err)
		return
	}

	environmentToReleaseToHTML := map[string]map[string]template.HTML{}
	for _, environment := range environments.Items {
		for i, release := range releases.Items {
			fmt.Printf("Processing release %s in environment %s\n", release.Name, environment.Name)

			currReleaseEnvironmentInfo, _ := h.releaseClient.GetEnvironmentRelease(ctx, environment.Name, release.Name)
			if currReleaseEnvironmentInfo == nil {
				continue
			}
			var prevReleaseEnvironmentInfo *status.EnvironmentRelease
			if i+1 < len(releases.Items) {
				prevReleaseEnvironmentInfo, err = h.releaseClient.GetEnvironmentRelease(ctx, environment.Name, releases.Items[i+1].Name)
			}

			releaseMap, ok := environmentToReleaseToHTML[environment.Name]
			if !ok {
				releaseMap = map[string]template.HTML{}
				environmentToReleaseToHTML[environment.Name] = releaseMap
			}

			changedComponents := ChangedComponents(currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo)

			changesList := ""
			if len(changedComponents) > 0 {
				changesList += fmt.Sprintf("<p>%d changes</p>", len(changedComponents))
				changesList += "<ul>\n"
				for _, changedComponent := range changedComponents.SortedList() {
					changesList += fmt.Sprintf("<li>%s</li>\n", changedComponent)
				}
				changesList += "</ul>\n"
			} else {
				changesList += "No changes"
				continue // don't display releases with no changes.
			}

			releaseMap[release.Name] = template.HTML(
				fmt.Sprintf(`
        <tr>
            <td>
                <a href=%q>%s</a>
            </td>
            <td>
                %s
            </td>
            <td>
                %s
            </td>
        </tr>
`,
					fmt.Sprintf("/http/aro-hcp/environmentreleases/%s/summary.html", url.PathEscape(GetEnvironmentReleaseName(environment.Name, release.Name))),
					release.Name,
					release.SHA,
					changesList,
				),
			)

		}
	}

	c.HTML(200, "http/aro-hcp/summary.html", gin.H{
		"environments":               environments,
		"releases":                   releases,
		"environmentToReleaseToHTML": environmentToReleaseToHTML,
	})
}

func ServeReleaseSummary(releaseClient client.ReleaseClient) func(c *gin.Context) {
	h := &htmlReleaseSummary{
		releaseClient: releaseClient,
	}
	return h.ServeGin
}

func ChangedComponents(currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo *status.EnvironmentRelease) set.Set[string] {
	changedComponents := set.Set[string]{}

	if prevReleaseEnvironmentInfo == nil {
		for _, currDeployedImageInfo := range currReleaseEnvironmentInfo.Images {
			changedComponents.Insert(currDeployedImageInfo.Name)
		}
		return changedComponents
	}

	for _, currDeployedImageInfo := range currReleaseEnvironmentInfo.Images {
		prevDeployedImageInfo := prevReleaseEnvironmentInfo.Images[currDeployedImageInfo.Name]
		if !reflect.DeepEqual(prevDeployedImageInfo.ImageInfo, currDeployedImageInfo.ImageInfo) {
			changedComponents.Insert(currDeployedImageInfo.Name)
		}
	}

	return changedComponents
}
