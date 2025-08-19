package release_webserver

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/apis/status"
	"github.com/openshift-online/service-status/pkg/aro/client"
	"k8s.io/utils/ptr"
)

type htmlEnvironmentReleaseSummary struct {
	releaseClient client.ReleaseClient
}

func (h *htmlEnvironmentReleaseSummary) ServeGin(c *gin.Context) {
	ctx := c.Request.Context()

	name := c.Param("name")
	environmentName, releaseName, found := SplitEnvironmentReleaseName(name)
	if !found {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("%q must be in format <environmentName>---<releaseName>", name)})
	}

	environmentReleaseInfo, err := h.releaseClient.GetEnvironmentRelease(ctx, environmentName, releaseName)
	if err != nil {
		c.String(500, "failed to get release environment: %v", err)
		return
	}

	release, err := h.releaseClient.GetRelease(ctx, environmentReleaseInfo.ReleaseName)
	if err != nil {
		c.String(500, "failed to get release %q: %v", environmentReleaseInfo.ReleaseName, err)
		return
	}

	var prevReleaseEnvironmentInfo *status.EnvironmentRelease
	if otherReleaseID := c.Query("from"); len(otherReleaseID) == 0 {
		// find the previous release. Very expensive, but probably ok
		releases, err := h.releaseClient.ListReleases(ctx)
		if err != nil {
			c.String(500, "failed to list releases: %v", err)
			return
		}

		for i, currRelease := range releases.Items {
			if currRelease.Name != releaseName {
				continue
			}
			if i+1 < len(releases.Items) {
				prevReleaseEnvironmentInfo, err = h.releaseClient.GetEnvironmentRelease(ctx, environmentName, releases.Items[i+1].Name)
			}
		}
	} else {
		otherEnvironmentName, otherReleaseName, _ := SplitEnvironmentReleaseName(otherReleaseID)
		var err error
		prevReleaseEnvironmentInfo, err = h.releaseClient.GetEnvironmentRelease(ctx, otherEnvironmentName, otherReleaseName)
		if err != nil {
			c.String(500, "failed to get from environment release: %v", err)
			return
		}
	}

	changedComponents := ChangedComponents(environmentReleaseInfo, prevReleaseEnvironmentInfo)
	changedNameToDetails := map[string]template.HTML{}
	if prevReleaseEnvironmentInfo != nil {
		diff, err := h.releaseClient.GetEnvironmentReleaseDiff(ctx, environmentReleaseInfo.Name, prevReleaseEnvironmentInfo.Name)
		if err != nil {
			fmt.Printf("failed to get diff for %q and %q: %v", environmentReleaseInfo.Name, prevReleaseEnvironmentInfo.Name, err)
		}
		for _, componentName := range changedComponents.UnsortedList() {
			var currImageDetails *status.Component
			var prevImageDetails *status.Component
			for _, imageDetails := range environmentReleaseInfo.Components {
				if imageDetails.Name == componentName {
					currImageDetails = imageDetails
					break
				}
			}
			for _, imageDetails := range prevReleaseEnvironmentInfo.Components {
				if imageDetails.Name == componentName {
					prevImageDetails = imageDetails
					break
				}
			}

			var componentDiff *status.ComponentDiff
			if diff != nil {
				componentDiff = diff.DifferentComponents[componentName]
			}
			detailsDiffHTML := htmlDetailsForComponentDiff(currImageDetails, prevImageDetails, prevReleaseEnvironmentInfo, componentDiff)
			changedNameToDetails[currImageDetails.Name] = template.HTML(detailsDiffHTML)
		}
	}

	imageNames := []string{}
	imageNameToDetails := map[string]template.HTML{}
	for _, imageDetails := range environmentReleaseInfo.Components {
		imageNames = append(imageNames, imageDetails.Name)
		detailsHTML := htmlDetailsForComponent(imageDetails)
		imageNameToDetails[imageDetails.Name] = template.HTML(detailsHTML)
	}
	sort.Strings(imageNames)

	prevEnvReleaseNameURLEscaped := ""
	if prevReleaseEnvironmentInfo != nil {
		prevEnvReleaseNameURLEscaped = url.PathEscape(prevReleaseEnvironmentInfo.Name)
	}

	allEnvironmentReleases, err := h.releaseClient.ListEnvironmentReleases(ctx)
	if err != nil {
		c.String(500, "failed to list allEnvironmentReleases: %v", err)
		return
	}

	c.HTML(200, "http/aro-hcp/environment-release.html", gin.H{
		"currEnvRelease":                environmentReleaseInfo,
		"prevEnvRelease":                prevReleaseEnvironmentInfo,
		"prevEnvReleaseNameURLEscaped":  prevEnvReleaseNameURLEscaped,
		"environmentName":               environmentReleaseInfo.Environment,
		"release":                       release,
		"changedComponentNames":         changedComponents.SortedList(),
		"changedComponentNameToDetails": changedNameToDetails,
		"componentNames":                imageNames,
		"componentNameToDetails":        imageNameToDetails,
		"allEnvironmentReleases":        allEnvironmentReleases.Items,
	})
}

func ServeEnvironmentReleaseSummary(releaseClient client.ReleaseClient) func(c *gin.Context) {
	h := &htmlEnvironmentReleaseSummary{
		releaseClient: releaseClient,
	}
	return h.ServeGin
}

func htmlDetailsForComponent(imageDetails *status.Component) string {
	imageAgeString := "Unknown age"
	imageTimeString := "Unknown time"
	if imageDetails.ImageCreationTime != nil {
		imageAgeString = humanize.RelTime(time.Now(), *imageDetails.ImageCreationTime, "INVALID", "old")
		imageTimeString = imageDetails.ImageCreationTime.Format(time.RFC3339)
	}

	imageSourceSHAString := imageDetails.SourceSHA
	if !strings.HasPrefix(imageDetails.SourceSHA, "ERROR") {
		switch {
		case len(imageDetails.SourceSHA) == 0:
			imageSourceSHAString = "MISSING"
		case imageDetails.PermanentURLForSourceSHA != nil && len(imageDetails.SourceSHA) > 0:
			imageSourceSHAString = fmt.Sprintf("<a href=%q>%s</a>", *imageDetails.PermanentURLForSourceSHA, imageDetails.SourceSHA)
		default:
			imageSourceSHAString = "DEFAULT"
		}
	}

	detailsHTML := fmt.Sprintf(`
		<h4><a target="_blank" href=%q>%s (%s)</a></h4>
        <details>
            <summary class="small mb-3">click to expand details</summary>
            <ul>
                <li>Pull Spec: %s</li>
                <ul>
                    <li>Image built %s</li>
                </ul>
                <li>Commit: %s</li>
            </ul>
        </details>
`,
		ptr.Deref(imageDetails.RepoURL, "MISSING"), imageDetails.Name, imageAgeString,
		fmt.Sprintf("%s/%s@%s", imageDetails.ImageInfo.Registry, imageDetails.ImageInfo.Repository, imageDetails.ImageInfo.Digest),
		imageTimeString,
		imageSourceSHAString,
	)

	return detailsHTML
}

func htmlDetailsForComponentDiff(currImageDetails, prevImageDetails *status.Component, prevReleaseEnvironmentInfo *status.EnvironmentRelease, diff *status.ComponentDiff) string {
	prevReleaseString := fmt.Sprintf("<a href=/http/aro-hcp/environmentreleases/%s/summary.html>%s</a>", prevReleaseEnvironmentInfo.Name, prevReleaseEnvironmentInfo.Name)

	imageAgeString := "Unknown age"
	imageTimeString := "Unknown time"
	if currImageDetails.ImageCreationTime != nil {
		imageAgeString = humanize.RelTime(time.Now(), *currImageDetails.ImageCreationTime, "INVALID", "old")
		imageTimeString = currImageDetails.ImageCreationTime.Format(time.RFC3339)
	}

	newerString := "Unknown amount newer"
	prevTimeString := "Unknown time"
	if prevImageDetails != nil && currImageDetails.ImageCreationTime != nil && prevImageDetails.ImageCreationTime != nil {
		newerString = humanize.RelTime(*currImageDetails.ImageCreationTime, *prevImageDetails.ImageCreationTime, "older", "newer than previous release")
		prevTimeString = prevImageDetails.ImageCreationTime.Format(time.RFC3339)
	}

	imageSourceSHAString := currImageDetails.SourceSHA
	if !strings.HasPrefix(currImageDetails.SourceSHA, "ERROR") {
		switch {
		case len(currImageDetails.SourceSHA) == 0:
			imageSourceSHAString = "MISSING"
		case currImageDetails.PermanentURLForSourceSHA != nil && len(currImageDetails.SourceSHA) > 0:
			imageSourceSHAString = fmt.Sprintf("<a href=%q>%s</a>", *currImageDetails.PermanentURLForSourceSHA, currImageDetails.SourceSHA)
		default:
			imageSourceSHAString = "DEFAULT"
		}
	}

	numberOfChangesString := "Unknown changes"
	diffLines := []string{}
	if diff != nil {
		if diff.NumberOfChanges >= 0 {
			numberOfChangesString = fmt.Sprintf("%d changes", diff.NumberOfChanges)
		}
		for _, change := range diff.Changes {
			switch {
			case change.ChangeType == "Unavailable":
				if change.Unavailable == nil {
					diffLines = append(diffLines, "<li>Unavailable, no info</li>")
				} else {
					diffLines = append(diffLines, fmt.Sprintf("<li>Unavailable, %s</li>", *change.Unavailable))
				}
			case change.ChangeType == "PRMerge":
				if change.PRMerge == nil {
					diffLines = append(diffLines, "<li>PR merge, no info</li>")
				} else {
					diffLines = append(diffLines, fmt.Sprintf("<li>%s <a target=\"_blank\" href=%q>#%d</a></li>",
						change.PRMerge.ChangeSummary,
						fmt.Sprintf("%s/pull/%d", ptr.Deref(currImageDetails.RepoURL, ""), change.PRMerge.PRNumber),
						change.PRMerge.PRNumber,
					))
				}
			}
		}
	}
	if len(ptr.Deref(currImageDetails.RepoURL, "")) > 0 && prevImageDetails != nil && len(prevImageDetails.SourceSHA) > 0 {
		diffLines = append(diffLines,
			fmt.Sprintf("<li><a target=\"_blank\" href=\"%s/compare/%s...%s\">Full changelog</a></li>",
				ptr.Deref(currImageDetails.RepoURL, ""),
				prevImageDetails.SourceSHA,
				currImageDetails.SourceSHA,
			))
	}

	detailsHTML := fmt.Sprintf(`
		<h4><a target="_blank" href=%q>%s (%s, %s, %s)</a></h4>
        <details>
            <summary class="small mb-3">click to expand details</summary>
            <ul>
                <li>Pull Spec: %s</li>
                <ul>
                    <li>%s</li>
                    <li>Image built %s</li>
                    <li>Previous image built %s</li>
                </ul>
                <li>Commit: %s</li>
                <li>Previous Release: %s</li>
				<li>Changes:</li>
				<ul>
					%s
				</ul>
            </ul>
        </details>
`,
		ptr.Deref(currImageDetails.RepoURL, "MISSING"), currImageDetails.Name, imageAgeString, newerString, numberOfChangesString,
		fmt.Sprintf("%s/%s@%s", currImageDetails.ImageInfo.Registry, currImageDetails.ImageInfo.Repository, currImageDetails.ImageInfo.Digest),
		newerString,
		imageTimeString,
		prevTimeString,
		imageSourceSHAString,
		prevReleaseString,
		strings.Join(diffLines, "\n\t\t\t\t\t"),
	)

	return detailsHTML
}
