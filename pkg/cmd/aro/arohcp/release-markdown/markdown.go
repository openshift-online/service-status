package release_markdown

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/dustin/go-humanize"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"k8s.io/utils/set"
)

func markdownForPertinentInfo(info *release_inspection.DeployedImageInfo) string {
	markdown := &strings.Builder{}
	fmt.Fprintf(markdown, "### [%s](%s)\n", info.Name, info.RepoLink)
	fmt.Fprintf(markdown, "* %s\n", info.RepoLink)
	fmt.Fprintf(markdown, "* Pull Spec: %s\n", stringOrErr(release_inspection.PullSpecFromContainerImage(info.ImageInfo)))
	if info.ImageCreationTime != nil {
		fmt.Fprintf(markdown, "  * Image built %s.\n", humanize.Time(*info.ImageCreationTime))
	} else {
		fmt.Fprintf(markdown, "  * Image built an unknown time ago.\n")
	}
	if strings.HasPrefix(info.SourceSHA, "ERROR") {
		fmt.Fprintf(markdown, "* Commit: %s\n", info.SourceSHA)
	} else {
		fmt.Fprintf(markdown, "* Commit: [%s](%s)\n", info.SourceSHA, info.PermLinkForSourceSHA)
	}
	fmt.Fprintf(markdown, "\n")

	return markdown.String()
}

func releaseEnvironmentMarkdown(currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo *release_inspection.ReleaseEnvironmentInfo) string {
	markdown := &strings.Builder{}
	fmt.Fprintf(markdown, "# Release %v\n\n", currReleaseEnvironmentInfo.ReleaseName)

	fmt.Fprintf(markdown, "## Diff\n\n")
	changedComponents := release_inspection.ChangedComponents(currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo)
	if len(changedComponents) == 0 {
		fmt.Fprintf(markdown, "*No Changes*\n\n")
	} else {
		for _, componentName := range set.KeySet(currReleaseEnvironmentInfo.DeployedImages).SortedList() {
			currInfo := currReleaseEnvironmentInfo.DeployedImages[componentName]
			if prevReleaseEnvironmentInfo != nil {
				prevInfo := prevReleaseEnvironmentInfo.DeployedImages[componentName]
				if reflect.DeepEqual(currInfo, prevInfo) {
					continue
				}
			}
			fmt.Fprintf(markdown, markdownForPertinentInfo(currInfo))
		}

	}

	fmt.Fprintf(markdown, "## Content\n\n")
	for _, componentName := range set.KeySet(currReleaseEnvironmentInfo.DeployedImages).SortedList() {
		info := currReleaseEnvironmentInfo.DeployedImages[componentName]
		fmt.Fprintf(markdown, markdownForPertinentInfo(info))
	}

	return markdown.String()
}

func allReleaseSummaryMarkdown(allReleasesInfo *release_inspection.ReleasesInfo) string {
	releaseSummaryMarkdown := &strings.Builder{}

	for _, environmentFilename := range allReleasesInfo.GetEnvironmentFilenames() {
		fmt.Fprintf(releaseSummaryMarkdown, "# %s Releases\n\n", strings.TrimSuffix(environmentFilename, ".json"))

		for i, currReleaseName := range allReleasesInfo.GetReleaseNames() {
			currReleaseInfo := allReleasesInfo.GetReleaseInfo(currReleaseName)
			currReleaseEnvironmentInfo := currReleaseInfo.GetInfoForEnvironment(environmentFilename)
			if currReleaseEnvironmentInfo == nil {
				continue
			}
			var prevReleaseEnvironmentInfo *release_inspection.ReleaseEnvironmentInfo
			if i > 0 {
				prevReleaseInfo := allReleasesInfo.GetReleaseInfo(allReleasesInfo.GetReleaseNames()[i-1])
				prevReleaseEnvironmentInfo = prevReleaseInfo.GetInfoForEnvironment(environmentFilename)
			}

			// TODO table
			changedComponents := release_inspection.ChangedComponents(currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo)
			fmt.Fprintf(releaseSummaryMarkdown, "* [%s](%s) %d changes (%v)\n",
				currReleaseEnvironmentInfo.ReleaseName,
				"TODO",
				len(changedComponents),
				strings.Join(changedComponents.SortedList(), ", "),
			)
		}
		fmt.Fprintf(releaseSummaryMarkdown, "\n")
	}

	return releaseSummaryMarkdown.String()
}
