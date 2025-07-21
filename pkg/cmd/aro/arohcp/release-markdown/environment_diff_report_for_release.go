package release_markdown

import (
	"fmt"
	"strings"
	"time"

	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"k8s.io/utils/set"
)

func environmentsBySameness(currReleaseInfo *release_inspection.ReleaseInfo) []set.Set[string] {
	congruentEnvironmentSets := []set.Set[string]{}

	usedEnvironments := set.New[string]()
	for _, environmentFilename := range currReleaseInfo.GetEnvironmentFilenames() {
		currEnvironmentInfo := currReleaseInfo.GetInfoForEnvironment(environmentFilename)
		currEnvironmentName := strings.TrimSuffix(currEnvironmentInfo.EnvironmentFilename, ".json")
		if usedEnvironments.Has(currEnvironmentName) {
			continue
		}

		otherEnvironmentInfos := []*release_inspection.ReleaseEnvironmentInfo{}
		for _, otherEnvironmentName := range currReleaseInfo.GetEnvironmentFilenames() {
			if otherEnvironmentName == environmentFilename {
				continue
			}
			otherEnvironmentInfos = append(otherEnvironmentInfos, currReleaseInfo.GetInfoForEnvironment(otherEnvironmentName))
		}

		congruentEnvironments := set.Set[string]{}
		congruentEnvironments.Insert(currEnvironmentName)
		congruentEnvironments.Insert(environmentsWithIdenticalImages(currEnvironmentInfo, otherEnvironmentInfos).UnsortedList()...)
		usedEnvironments.Insert(congruentEnvironments.UnsortedList()...)

		congruentEnvironmentSets = append(congruentEnvironmentSets, congruentEnvironments)
	}

	return congruentEnvironmentSets
}

func releaseEnvironmentSummaryMarkdown(currReleaseInfo *release_inspection.ReleaseInfo) string {
	releaseSummaryMarkdown := &strings.Builder{}
	fmt.Fprintf(releaseSummaryMarkdown, "# %s Release\n\n", currReleaseInfo.ReleaseName)

	wrote := false
	congruentEnvironments := environmentsBySameness(currReleaseInfo)
	environmentToCongruentEnvironments := map[string]set.Set[string]{}
	for _, congruentEnvironment := range congruentEnvironments {
		for currEnvironment := range congruentEnvironment {
			environmentToCongruentEnvironments[currEnvironment] = congruentEnvironment
		}
		if len(congruentEnvironment) == 1 {
			continue
		}
		fmt.Fprintf(releaseSummaryMarkdown, "* %s environments are the same\n", strings.Join(congruentEnvironment.SortedList(), ", "))
		wrote = true
	}
	if wrote {
		fmt.Fprintf(releaseSummaryMarkdown, "\n")
	}

	handledEnvironments := set.Set[string]{}
	for _, environmentFilename := range currReleaseInfo.GetEnvironmentFilenames() {
		currEnvironmentInfo := currReleaseInfo.GetInfoForEnvironment(environmentFilename)
		currEnvironmentName := strings.TrimSuffix(currEnvironmentInfo.EnvironmentFilename, ".json")
		if handledEnvironments.Has(currEnvironmentName) {
			continue
		}

		otherEnvironmentInfos := []*release_inspection.ReleaseEnvironmentInfo{}
		for _, otherEnvironmentName := range currReleaseInfo.GetEnvironmentFilenames() {
			if otherEnvironmentName == environmentFilename {
				continue
			}
			otherEnvironmentInfos = append(otherEnvironmentInfos, currReleaseInfo.GetInfoForEnvironment(otherEnvironmentName))
		}

		environmentMarkdown := markdownOfCurrentEnvironmentToOthers(currEnvironmentInfo, otherEnvironmentInfos, environmentToCongruentEnvironments)
		fmt.Fprintf(releaseSummaryMarkdown, environmentMarkdown)

		handledEnvironments.Insert(environmentToCongruentEnvironments[currEnvironmentName].UnsortedList()...)
	}

	return releaseSummaryMarkdown.String()
}

func environmentsWithIdenticalImages(currEnvironmentInfo *release_inspection.ReleaseEnvironmentInfo, otherEnvironmentInfos []*release_inspection.ReleaseEnvironmentInfo) set.Set[string] {
	sameEnvironments := set.Set[string]{}

	for _, otherEnvironmentInfo := range otherEnvironmentInfos {
		allDeployedImages := set.Set[string]{}
		allDeployedImages.Insert(set.KeySet(currEnvironmentInfo.DeployedImages).UnsortedList()...)
		allDeployedImages.Insert(set.KeySet(otherEnvironmentInfo.DeployedImages).UnsortedList()...)

		sameImages := set.Set[string]{}
		differentImageDetails := map[string]string{}
		currMissingImages := set.Set[string]{}
		otherMissingImages := set.Set[string]{}
		for _, deployedImageName := range allDeployedImages.SortedList() {
			currDeployedImageInfo := currEnvironmentInfo.DeployedImages[deployedImageName]
			otherDeployedImageInfo := otherEnvironmentInfo.DeployedImages[deployedImageName]
			if currDeployedImageInfo == nil {
				currMissingImages.Insert(deployedImageName)
				continue
			}
			if otherDeployedImageInfo == nil {
				otherMissingImages.Insert(deployedImageName)
				continue
			}

			if currDeployedImageInfo.ImageInfo == nil {
				differentImageDetails[deployedImageName] = "currDeployedImageInfo is missing, assuming different"
				continue
			}
			if otherDeployedImageInfo.ImageInfo == nil {
				differentImageDetails[deployedImageName] = "otherDeployedImageInfo is missing, assuming different"
				continue
			}

			currImageDigest := currDeployedImageInfo.ImageInfo.Digest
			otherImageDigest := otherDeployedImageInfo.ImageInfo.Digest
			if currImageDigest == otherImageDigest {
				sameImages.Insert(deployedImageName)
				continue
			}
			differentImageDetails[deployedImageName] = "Different, but missing details"

			if currDeployedImageInfo.ImageCreationTime == nil || otherDeployedImageInfo.ImageCreationTime == nil {
				continue
			}
		}
		differentImages := set.KeySet(differentImageDetails)

		otherEnvironmentName := strings.TrimSuffix(otherEnvironmentInfo.EnvironmentFilename, ".json")
		if len(differentImages) == 0 && len(currMissingImages) == 0 && len(otherMissingImages) == 0 {
			sameEnvironments.Insert(otherEnvironmentName)
			continue
		}
	}

	return sameEnvironments
}

func markdownOfCurrentEnvironmentToOthers(currEnvironmentInfo *release_inspection.ReleaseEnvironmentInfo, otherEnvironmentInfos []*release_inspection.ReleaseEnvironmentInfo, environmentToCongruentEnvironments map[string]set.Set[string]) string {
	environmentSummaryMarkdown := &strings.Builder{}
	currEnvironmentName := strings.TrimSuffix(currEnvironmentInfo.EnvironmentFilename, ".json")
	congruentEnvironmentsForCurr := environmentToCongruentEnvironments[currEnvironmentName]
	if len(congruentEnvironmentsForCurr) == 1 {
		fmt.Fprintf(environmentSummaryMarkdown, "## %s Environment\n", strings.Join(congruentEnvironmentsForCurr.SortedList(), ", "))
	} else {
		fmt.Fprintf(environmentSummaryMarkdown, "## %s Environments\n", strings.Join(congruentEnvironmentsForCurr.SortedList(), ", "))
	}

	checkedEnvironments := set.Set[string]{}
	for _, otherEnvironmentInfo := range otherEnvironmentInfos {
		otherEnvironmentName := strings.TrimSuffix(otherEnvironmentInfo.EnvironmentFilename, ".json")
		if congruentEnvironmentsForCurr.Has(otherEnvironmentName) {
			continue
		}
		if checkedEnvironments.Has(otherEnvironmentName) {
			continue
		}

		allDeployedImages := set.Set[string]{}
		allDeployedImages.Insert(set.KeySet(currEnvironmentInfo.DeployedImages).UnsortedList()...)
		allDeployedImages.Insert(set.KeySet(otherEnvironmentInfo.DeployedImages).UnsortedList()...)

		sameImages := set.Set[string]{}
		differentImageDetails := map[string]string{}
		currMissingImages := set.Set[string]{}
		otherMissingImages := set.Set[string]{}
		for _, deployedImageName := range allDeployedImages.SortedList() {
			currDeployedImageInfo := currEnvironmentInfo.DeployedImages[deployedImageName]
			otherDeployedImageInfo := otherEnvironmentInfo.DeployedImages[deployedImageName]
			if currDeployedImageInfo == nil {
				currMissingImages.Insert(deployedImageName)
				continue
			}
			if otherDeployedImageInfo == nil {
				otherMissingImages.Insert(deployedImageName)
				continue
			}

			if currDeployedImageInfo.ImageInfo == nil {
				differentImageDetails[deployedImageName] = "currDeployedImageInfo is missing, assuming different"
				continue
			}
			if otherDeployedImageInfo.ImageInfo == nil {
				differentImageDetails[deployedImageName] = "otherDeployedImageInfo is missing, assuming different"
				continue
			}

			currImageDigest := currDeployedImageInfo.ImageInfo.Digest
			otherImageDigest := otherDeployedImageInfo.ImageInfo.Digest
			if currImageDigest == otherImageDigest {
				sameImages.Insert(deployedImageName)
				continue
			}

			switch {
			case currDeployedImageInfo.ImageCreationTime == nil || otherDeployedImageInfo.ImageCreationTime == nil:
				differentImageDetails[deployedImageName] = "is different, but missing details"

			case currDeployedImageInfo.ImageCreationTime.After(*otherDeployedImageInfo.ImageCreationTime):
				newerDuration := currDeployedImageInfo.ImageCreationTime.Sub(*otherDeployedImageInfo.ImageCreationTime)
				if newerDuration < 24*time.Hour {
					differentImageDetails[deployedImageName] = fmt.Sprintf("is %v newer", newerDuration.Round(time.Hour))
				} else {
					days := int64(newerDuration / (24 * time.Hour))
					differentImageDetails[deployedImageName] = fmt.Sprintf("is %v days newer", days)
				}

			case otherDeployedImageInfo.ImageCreationTime.After(*currDeployedImageInfo.ImageCreationTime):
				olderDuration := otherDeployedImageInfo.ImageCreationTime.Sub(*currDeployedImageInfo.ImageCreationTime)
				if olderDuration < 24*time.Hour {
					differentImageDetails[deployedImageName] = fmt.Sprintf("is %v older", olderDuration.Round(time.Hour))
				} else {
					days := int64(olderDuration / (24 * time.Hour))
					differentImageDetails[deployedImageName] = fmt.Sprintf("is %v days older", days)
				}

			default:
				differentImageDetails[deployedImageName] = "is different, but missing details (default case)"
			}
		}
		differentImages := set.KeySet(differentImageDetails)

		otherEnvironmentAndCongruents := environmentToCongruentEnvironments[otherEnvironmentName]
		checkedEnvironments.Insert(otherEnvironmentAndCongruents.UnsortedList()...)
		otherEnvironmentAndCongruentsNames := strings.Join(otherEnvironmentAndCongruents.SortedList(), ", ")
		if len(differentImages) == 0 && len(currMissingImages) == 0 && len(otherMissingImages) == 0 {
			fmt.Fprintf(environmentSummaryMarkdown, "### %s Environment (same)\n", otherEnvironmentAndCongruentsNames)
			continue
		}

		fmt.Fprintf(environmentSummaryMarkdown, "### %s Environments\n", otherEnvironmentAndCongruentsNames)
		if len(currMissingImages) > 0 {
			fmt.Fprintf(environmentSummaryMarkdown, "* %s is missing images: %v\n", currEnvironmentName, strings.Join(currMissingImages.SortedList(), ", "))
		}
		if len(otherMissingImages) > 0 {
			fmt.Fprintf(environmentSummaryMarkdown, "* %s are missing images: %v\n", otherEnvironmentAndCongruentsNames, strings.Join(otherMissingImages.SortedList(), ", "))
		}
		for _, differentImageName := range differentImages.SortedList() {
			fmt.Fprintf(environmentSummaryMarkdown, "* %s %s\n", differentImageName, differentImageDetails[differentImageName])
		}

		fmt.Fprintf(environmentSummaryMarkdown, "\n")
	}

	fmt.Fprintf(environmentSummaryMarkdown, "\n")

	return environmentSummaryMarkdown.String()
}
