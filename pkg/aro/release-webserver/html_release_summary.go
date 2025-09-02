package release_webserver

import (
	"fmt"
	"html/template"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/apis/status"
	"github.com/openshift-online/service-status/pkg/aro/client"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
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
	environmentToSummaryHTML := map[string]template.HTML{}
	for _, environment := range environments.Items {
		for i, release := range releases.Items {
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
            <td class="text-monospace">
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

			if len(environmentToSummaryHTML[environment.Name]) == 0 { // first one is the summary one
				environmentToSummaryHTML[environment.Name] = summaryForEnvironment(currReleaseEnvironmentInfo)
			}
		}
	}

	c.HTML(200, "http/aro-hcp/summary.html", gin.H{
		"environments":               environments,
		"releases":                   releases,
		"environmentToReleaseToHTML": environmentToReleaseToHTML,
		"environmentToSummaryHTML":   environmentToSummaryHTML,
	})
}

func summaryForEnvironment(environmentRelease *status.EnvironmentRelease) template.HTML {
	now := time.Now()
	lines := []string{}
	if environmentRelease.Environment == "int" {
		for _, componentName := range set.KeySet(environmentRelease.Components).SortedList() {
			component := environmentRelease.Components[componentName]
			if component.ImageCreationTime == nil {
				continue
			}
			acceptableLatency := release_inspection.HardcodedComponents[component.Name].LatencyThreshold
			if acceptableLatency == 0 {
				continue
			}

			daysOld := now.Sub(*component.ImageCreationTime) / (24 * time.Hour)
			roughWorkingDuration := now.Sub(*component.ImageCreationTime) - daysOld
			if roughWorkingDuration > acceptableLatency {
				lines = append(lines,
					fmt.Sprintf("<li><b>%s</b> needs to be updated.  It is about %d days old and should be updated every %d days.</li>",
						component.Name, roughWorkingDuration/(24*time.Hour), acceptableLatency/(24*time.Hour)),
				)
			}
		}
	}

	if len(lines) == 0 {
		return "All images are up to date"
	}

	return template.HTML(fmt.Sprintf(`
<ul>
    %s
</ul>
`,
		strings.Join(lines, "\n    ")))
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
		for _, currComponent := range currReleaseEnvironmentInfo.Components {
			changedComponents.Insert(currComponent.Name)
		}
		return changedComponents
	}

	for _, currComponent := range currReleaseEnvironmentInfo.Components {
		prevComponent := prevReleaseEnvironmentInfo.Components[currComponent.Name]
		if !reflect.DeepEqual(prevComponent.ImageInfo, currComponent.ImageInfo) {
			changedComponents.Insert(currComponent.Name)
		}
	}

	return changedComponents
}
