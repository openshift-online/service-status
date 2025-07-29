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

	// find the previous release. Very expensive, but probably ok
	releases, err := h.releaseClient.ListReleases(ctx)
	if err != nil {
		c.String(500, "failed to list releases: %v", err)
		return
	}

	var prevReleaseEnvironmentInfo *status.EnvironmentRelease
	for i, currRelease := range releases.Items {
		if currRelease.Name != releaseName {
			continue
		}
		if i+1 < len(releases.Items) {
			prevReleaseEnvironmentInfo, err = h.releaseClient.GetEnvironmentRelease(ctx, environmentName, releases.Items[i+1].Name)
		}
	}
	changedComponents := ChangedComponents(environmentReleaseInfo, prevReleaseEnvironmentInfo)
	changedNameToDetails := map[string]template.HTML{}
	if prevReleaseEnvironmentInfo != nil {
		for _, changedImageName := range changedComponents.UnsortedList() {
			var currImageDetails *status.DeployedImageInfo
			var prevImageDetails *status.DeployedImageInfo
			for _, imageDetails := range environmentReleaseInfo.Images {
				if imageDetails.Name == changedImageName {
					currImageDetails = imageDetails
					break
				}
			}
			for _, imageDetails := range prevReleaseEnvironmentInfo.Images {
				if imageDetails.Name == changedImageName {
					prevImageDetails = imageDetails
					break
				}
			}
			detailsDiffHTML := htmlDetailsForComponentDiff(currImageDetails, prevImageDetails, prevReleaseEnvironmentInfo)
			changedNameToDetails[currImageDetails.Name] = template.HTML(detailsDiffHTML)
		}
	}

	imageNames := []string{}
	imageNameToDetails := map[string]template.HTML{}
	for _, imageDetails := range environmentReleaseInfo.Images {
		imageNames = append(imageNames, imageDetails.Name)
		detailsHTML := htmlDetailsForComponent(imageDetails)
		imageNameToDetails[imageDetails.Name] = template.HTML(detailsHTML)
	}
	sort.Strings(imageNames)

	prevEnvReleaseNameURLEscaped := ""
	if prevReleaseEnvironmentInfo != nil {
		prevEnvReleaseNameURLEscaped = url.PathEscape(prevReleaseEnvironmentInfo.Name)
	}
	c.HTML(200, "http/aro-hcp/environment-release.html", gin.H{
		"currEnvRelease":               environmentReleaseInfo,
		"prevEnvRelease":               prevReleaseEnvironmentInfo,
		"prevEnvReleaseNameURLEscaped": prevEnvReleaseNameURLEscaped,
		"environmentName":              environmentReleaseInfo.Environment,
		"release":                      release,
		"changedImageNames":            changedComponents.SortedList(),
		"changedImageNameToDetails":    changedNameToDetails,
		"imageNames":                   imageNames,
		"imageNameToDetails":           imageNameToDetails,
	})
}

func ServeEnvironmentReleaseSummary(releaseClient client.ReleaseClient) func(c *gin.Context) {
	h := &htmlEnvironmentReleaseSummary{
		releaseClient: releaseClient,
	}
	return h.ServeGin
}

func htmlDetailsForComponent(imageDetails *status.DeployedImageInfo) string {
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

func htmlDetailsForComponentDiff(currImageDetails, prevImageDetails *status.DeployedImageInfo, prevReleaseEnvironmentInfo *status.EnvironmentRelease) string {
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

	detailsHTML := fmt.Sprintf(`
		<h4><a target="_blank" href=%q>%s (%s, %s)</a></h4>
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
            </ul>
        </details>
`,
		ptr.Deref(currImageDetails.RepoURL, "MISSING"), currImageDetails.Name, imageAgeString, newerString,
		fmt.Sprintf("%s/%s@%s", currImageDetails.ImageInfo.Registry, currImageDetails.ImageInfo.Repository, currImageDetails.ImageInfo.Digest),
		newerString,
		imageTimeString,
		prevTimeString,
		imageSourceSHAString,
		prevReleaseString,
	)

	return detailsHTML
}
