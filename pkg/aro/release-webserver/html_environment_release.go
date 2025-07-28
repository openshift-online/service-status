package release_webserver

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
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

	imageNames := []string{}
	imageNameToDetails := map[string]template.HTML{}
	for _, imageDetails := range environmentReleaseInfo.Images {
		imageNames = append(imageNames, imageDetails.Name)
		imageAgeString := "Unknown age"
		if imageDetails.ImageCreationTime != nil {
			imageAgeString = fmt.Sprintf("%s.", humanize.Time(*imageDetails.ImageCreationTime))
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
		<h3>%s (%s)</h3>
        <details>
            <summary>click to expand details</summary>
            <ul>
                <li><a href=%q>%s</a></li>
                <li>Pull Spec: %s</li>
                <ul>
                    <li>Image built %s</li>
                </ul>
                <li>Commit: %s</li>
            </ul>
        </details>
`,
			imageDetails.Name, imageAgeString,
			ptr.Deref(imageDetails.RepoURL, "MISSING"), ptr.Deref(imageDetails.RepoURL, "MISSING"),
			fmt.Sprintf("%s/%s@sha256:%s", imageDetails.ImageInfo.Registry, imageDetails.ImageInfo.Repository, imageDetails.ImageInfo.Digest),
			imageAgeString,
			imageSourceSHAString,
		)

		imageNameToDetails[imageDetails.Name] = template.HTML(detailsHTML)
	}
	sort.Strings(imageNames)

	c.HTML(200, "http/aro-hcp/environment-release.html", gin.H{
		"environmentName":    environmentReleaseInfo.Environment,
		"release":            release,
		"imageNames":         imageNames,
		"imageNameToDetails": imageNameToDetails,
	})
}

func ServeEnvironmentReleaseSummary(releaseClient client.ReleaseClient) func(c *gin.Context) {
	h := &htmlEnvironmentReleaseSummary{
		releaseClient: releaseClient,
	}
	return h.ServeGin
}
