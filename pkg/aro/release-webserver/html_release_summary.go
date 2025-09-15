package release_webserver

import (
	"fmt"
	"html/template"
	"net/url"
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

	environmentToEnvironmentReleases := map[string]*status.EnvironmentReleaseList{}
	for _, environment := range environments.Items {
		environmentReleases, err := h.releaseClient.ListEnvironmentReleasesForEnvironment(ctx, environment.Name)
		if err != nil {
			c.String(500, "failed to list environments: %v", err)
			return
		}
		environmentToEnvironmentReleases[environment.Name] = environmentReleases
	}

	environmentReleaseToHTML := map[string]template.HTML{}
	environmentToEnvironmentReleaseNames := map[string][]string{}
	environmentToSummaryHTML := map[string]template.HTML{}
	for _, environment := range environments.Items {
		environmentReleases := environmentToEnvironmentReleases[environment.Name]

		for i, currReleaseEnvironmentInfo := range environmentReleases.Items {
			environmentToEnvironmentReleaseNames[environment.Name] = append(environmentToEnvironmentReleaseNames[environment.Name], currReleaseEnvironmentInfo.Name)

			var prevReleaseEnvironmentInfo *status.EnvironmentRelease
			if i+1 < len(environmentReleases.Items) {
				prevReleaseEnvironmentInfo, err = h.releaseClient.GetEnvironmentRelease(ctx, environment.Name, environmentReleases.Items[i+1].ReleaseName)
			}

			changedComponents := release_inspection.ChangedComponents(&currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo)

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
			}

			environmentReleaseToHTML[currReleaseEnvironmentInfo.Name] = perEnvironmentReleaseRow(currReleaseEnvironmentInfo, changesList, environmentToEnvironmentReleases)
		}

		if len(environmentToSummaryHTML[environment.Name]) == 0 { // first one is the summary one
			environmentToSummaryHTML[environment.Name] = summaryForEnvironment(environment.Name, environmentToEnvironmentReleases)
		}

	}

	c.HTML(200, "http/aro-hcp/summary.html", gin.H{
		"environments":                         environments,
		"environmentToEnvironmentReleaseNames": environmentToEnvironmentReleaseNames,
		"environmentReleaseToHTML":             environmentReleaseToHTML,
		"environmentToSummaryHTML":             environmentToSummaryHTML,
	})
}

func perEnvironmentReleaseRow(currReleaseEnvironmentInfo status.EnvironmentRelease, changesList string, environmentToEnvironmentReleases map[string]*status.EnvironmentReleaseList) template.HTML {
	jobRunsHTML := `<table  id="{{$environment}}_table" class="table text-nowrap small mb-3">
          <colgroup>
            <col style="width: 100px;">
            <col style="width: 200px;">
          </colgroup>
`
	jobRunsHTML += htmlRowsForCIResults(currReleaseEnvironmentInfo.BlockingJobRunResults)
	jobRunsHTML += htmlRowsForCIResults(currReleaseEnvironmentInfo.InformingJobRunResults)
	jobRunsHTML += `    </table>
`
	matchingReleasesHTML := htmlCellMatchingReleases(currReleaseEnvironmentInfo, environmentToEnvironmentReleases)

	return template.HTML(
		fmt.Sprintf(`
        <tr>
            <td class="text-monospace">
                <a href=%q>%s</a>
            </td>
            <td >
                %s
            </td>
            <td>
                %s
            </td>
            <td>
                %s
            </td>
        </tr>
`,
			fmt.Sprintf("/http/aro-hcp/environmentreleases/%s/summary.html", url.PathEscape(release_inspection.MakeEnvironmentReleaseName(currReleaseEnvironmentInfo.Environment, currReleaseEnvironmentInfo.ReleaseName))),
			currReleaseEnvironmentInfo.ReleaseName,
			jobRunsHTML,
			matchingReleasesHTML,
			changesList,
		),
	)
}

func htmlCellMatchingReleases(currReleaseEnvironmentInfo status.EnvironmentRelease, environmentToEnvironmentReleases map[string]*status.EnvironmentReleaseList) string {
	retMatchingReleases := []string{}

	environmentsToCheck := []string{"int", "stg", "prod"}
	for _, environmentToCheck := range environmentsToCheck {
		environmentReleases := environmentToEnvironmentReleases[environmentToCheck]
		for _, comparisonEnvironmentRelease := range environmentReleases.Items {
			if comparisonEnvironmentRelease.Name == currReleaseEnvironmentInfo.Name {
				continue
			}
			currChangedComponents := release_inspection.ChangedComponents(&currReleaseEnvironmentInfo, &comparisonEnvironmentRelease)
			if len(currChangedComponents) == 0 {
				retMatchingReleases = append(retMatchingReleases,
					fmt.Sprintf(`
					<li>
					    <a href=%q>%s</a>
					</li>`,
						fmt.Sprintf("/http/aro-hcp/environmentreleases/%s/summary.html",
							url.PathEscape(comparisonEnvironmentRelease.Name)),
						comparisonEnvironmentRelease.Name))
			}
		}
	}
	if len(retMatchingReleases) == 0 {
		return ""
	}

	retHTML := fmt.Sprintf(`
				<ul class="table text-nowrap small mb-3">
				%s
				</ul>`, strings.Join(retMatchingReleases, "\n"))
	return retHTML
}

func htmlRowsForCIResults(ciResults map[string][]status.JobRunResults) string {
	retHTML := ""
	for _, variantName := range set.KeySet(ciResults).SortedList() {
		currResults := ciResults[variantName]
		currResultsHTML := ""
		for i, currResult := range currResults {
			currResultHTML := ""
			if i > 0 && i%10 == 0 {
				currResultsHTML += "<br/>\n"
			}
			if currResult.OverallResult == status.JobSucceeded {
				currResultHTML = fmt.Sprintf(`<a href=%q class="text-success">%s</a>`, currResult.URL, currResult.OverallResult)
			} else {
				currResultHTML = fmt.Sprintf(`<a href=%q class="text-danger" >%s</a>`, currResult.URL, currResult.OverallResult)
			}
			currResultsHTML += fmt.Sprintf(` %s`, currResultHTML)
		}
		variantHTML := fmt.Sprintf(`
        <tr>
            <td>
                %s
            </td>
            <td style="text-align: left;" class="text-monospace">
                %s
            </td>
        </tr>
`, variantName, currResultsHTML)

		retHTML += variantHTML
	}
	return retHTML
}

func summaryForEnvironment(environmentName string, environmentToEnvironmentReleases map[string]*status.EnvironmentReleaseList) template.HTML {
	now := time.Now()
	if environmentName == "int" {
		if len(environmentToEnvironmentReleases[environmentName].Items) == 0 {
			return "No releases found."
		}

		lines := []string{}
		environmentRelease := environmentToEnvironmentReleases[environmentName].Items[0]
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

		if len(lines) == 0 {
			return "All images are up to date."
		}
		return template.HTML(fmt.Sprintf(`
		    <ul>
    		    %s
		    </ul>
`,
			strings.Join(lines, "\n    ")))
	}

	if environmentName == "stg" {
		lines := []string{}
		stageEnvironmentReleases := environmentToEnvironmentReleases[environmentName]
		if len(stageEnvironmentReleases.Items) > 0 {
			stageEnvironmentRelease := stageEnvironmentReleases.Items[0]
			minChangedRelease, minChangedComponents := findMatchingEnvironmentRelease(environmentToEnvironmentReleases["int"], &stageEnvironmentRelease)
			if len(minChangedComponents) > 0 {
				lines = append(lines,
					fmt.Sprintf("<li><b>%s</b> was never tested in integration. Closest release is %s which differs by %s.</li>",
						stageEnvironmentRelease.Name, minChangedRelease.Name, strings.Join(minChangedComponents.SortedList(), ", ")),
				)
			}
		}

		if len(lines) == 0 {
			return "Latest stage release was first tested in integration."
		}
		return template.HTML(fmt.Sprintf(`
		    <ul>
		        %s
		    </ul>
`,
			strings.Join(lines, "\n    ")))
	}

	if environmentName == "prod" {
		lines := []string{}
		prodEnvironmentReleases := environmentToEnvironmentReleases[environmentName]
		if len(prodEnvironmentReleases.Items) > 0 {
			prodEnvironmentRelease := prodEnvironmentReleases.Items[0]
			minChangedRelease, minChangedComponents := findMatchingEnvironmentRelease(environmentToEnvironmentReleases["stg"], &prodEnvironmentRelease)
			if len(minChangedComponents) > 0 {
				lines = append(lines,
					fmt.Sprintf("<li><b>%s</b> was never tested in stage. Closest release is %s which differs by %s.</li>",
						prodEnvironmentRelease.Name, minChangedRelease.Name, strings.Join(minChangedComponents.SortedList(), ", ")),
				)
			}
		}

		if len(lines) == 0 {
			return "Latest production release was first tested in staging."
		}
		return template.HTML(fmt.Sprintf(`
		    <ul>
		        %s
		    </ul>
`,
			strings.Join(lines, "\n    ")))
	}

	return ""
}

func findMatchingEnvironmentRelease(haystack *status.EnvironmentReleaseList, needle *status.EnvironmentRelease) (*status.EnvironmentRelease, set.Set[string]) {
	var minChangedComponents set.Set[string]
	var minChangedRelease *status.EnvironmentRelease
	for i := range haystack.Items {
		currEnvironmentRelease := haystack.Items[i]

		currChangedComponents := release_inspection.ChangedComponents(&currEnvironmentRelease, needle)
		if len(currChangedComponents) == 0 {
			return &currEnvironmentRelease, nil
		}

		if minChangedComponents.Len() == 0 || len(currChangedComponents) < minChangedComponents.Len() {
			minChangedComponents = currChangedComponents
			minChangedRelease = &currEnvironmentRelease
		}
	}

	return minChangedRelease, minChangedComponents
}

func ServeReleaseSummary(releaseClient client.ReleaseClient) func(c *gin.Context) {
	h := &htmlReleaseSummary{
		releaseClient: releaseClient,
	}
	return h.ServeGin
}
