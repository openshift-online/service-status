package release_markdown

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/utils/set"
)

func environmentsBySameness(currReleaseInfo *releaseInfo) []set.Set[string] {
	congruentEnvironmentSets := []set.Set[string]{}

	usedEnvironments := set.New[string]()
	for _, environmentFilename := range currReleaseInfo.getEnvironmentFilenames() {
		currEnvironmentInfo := currReleaseInfo.getInfoForEnvironment(environmentFilename)
		currEnvironmentName := strings.TrimSuffix(currEnvironmentInfo.environmentFilename, ".json")
		if usedEnvironments.Has(currEnvironmentName) {
			continue
		}

		otherEnvironmentInfos := []*releaseEnvironmentInfo{}
		for _, otherEnvironmentName := range currReleaseInfo.getEnvironmentFilenames() {
			if otherEnvironmentName == environmentFilename {
				continue
			}
			otherEnvironmentInfos = append(otherEnvironmentInfos, currReleaseInfo.environmentToEnvironmentInfo[otherEnvironmentName])
		}

		congruentEnvironments := set.Set[string]{}
		congruentEnvironments.Insert(currEnvironmentName)
		congruentEnvironments.Insert(environmentsWithIdenticalImages(currEnvironmentInfo, otherEnvironmentInfos).UnsortedList()...)
		usedEnvironments.Insert(congruentEnvironments.UnsortedList()...)

		congruentEnvironmentSets = append(congruentEnvironmentSets, congruentEnvironments)
	}

	return congruentEnvironmentSets
}

func allReleasesEnvironmentSummaryMarkdown(allReleasesInfo *releasesInfo) string {
	releaseSummaryMarkdown := &strings.Builder{}

	for _, currReleaseName := range allReleasesInfo.getReleaseNames() {
		fmt.Fprintf(releaseSummaryMarkdown, "# %s Release\n\n", currReleaseName)

		currReleaseInfo := allReleasesInfo.getReleaseInfo(currReleaseName)
		for _, environmentFilename := range currReleaseInfo.getEnvironmentFilenames() {
			currEnvironmentInfo := currReleaseInfo.getInfoForEnvironment(environmentFilename)
			otherEnvironmentInfos := []*releaseEnvironmentInfo{}
			for _, otherEnvironmentName := range currReleaseInfo.getEnvironmentFilenames() {
				if otherEnvironmentName == environmentFilename {
					continue
				}
				otherEnvironmentInfos = append(otherEnvironmentInfos, currReleaseInfo.environmentToEnvironmentInfo[otherEnvironmentName])
			}

			environmentMarkdown := markdownOfCurrentEnvironmentToOthers(currEnvironmentInfo, otherEnvironmentInfos, nil)
			fmt.Fprintf(releaseSummaryMarkdown, environmentMarkdown)
		}

	}

	return releaseSummaryMarkdown.String()
}

func releaseEnvironmentSummaryMarkdown(currReleaseInfo *releaseInfo) string {
	releaseSummaryMarkdown := &strings.Builder{}
	fmt.Fprintf(releaseSummaryMarkdown, "# %s Release\n\n", currReleaseInfo.releaseName)

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
	for _, environmentFilename := range currReleaseInfo.getEnvironmentFilenames() {
		currEnvironmentInfo := currReleaseInfo.getInfoForEnvironment(environmentFilename)
		currEnvironmentName := strings.TrimSuffix(currEnvironmentInfo.environmentFilename, ".json")
		if handledEnvironments.Has(currEnvironmentName) {
			continue
		}

		otherEnvironmentInfos := []*releaseEnvironmentInfo{}
		for _, otherEnvironmentName := range currReleaseInfo.getEnvironmentFilenames() {
			if otherEnvironmentName == environmentFilename {
				continue
			}
			otherEnvironmentInfos = append(otherEnvironmentInfos, currReleaseInfo.environmentToEnvironmentInfo[otherEnvironmentName])
		}

		environmentMarkdown := markdownOfCurrentEnvironmentToOthers(currEnvironmentInfo, otherEnvironmentInfos, environmentToCongruentEnvironments)
		fmt.Fprintf(releaseSummaryMarkdown, environmentMarkdown)

		handledEnvironments.Insert(environmentToCongruentEnvironments[currEnvironmentName].UnsortedList()...)
	}

	return releaseSummaryMarkdown.String()
}

func environmentsWithIdenticalImages(currEnvironmentInfo *releaseEnvironmentInfo, otherEnvironmentInfos []*releaseEnvironmentInfo) set.Set[string] {
	sameEnvironments := set.Set[string]{}

	for _, otherEnvironmentInfo := range otherEnvironmentInfos {
		allDeployedImages := set.Set[string]{}
		allDeployedImages.Insert(set.KeySet(currEnvironmentInfo.pertinentInfo.deployedImages).UnsortedList()...)
		allDeployedImages.Insert(set.KeySet(otherEnvironmentInfo.pertinentInfo.deployedImages).UnsortedList()...)

		sameImages := set.Set[string]{}
		differentImageDetails := map[string]string{}
		currMissingImages := set.Set[string]{}
		otherMissingImages := set.Set[string]{}
		for _, deployedImageName := range allDeployedImages.SortedList() {
			currDeployedImageInfo := currEnvironmentInfo.pertinentInfo.deployedImages[deployedImageName]
			otherDeployedImageInfo := otherEnvironmentInfo.pertinentInfo.deployedImages[deployedImageName]
			if currDeployedImageInfo == nil {
				currMissingImages.Insert(deployedImageName)
				continue
			}
			if otherDeployedImageInfo == nil {
				otherMissingImages.Insert(deployedImageName)
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

		otherEnvironmentName := strings.TrimSuffix(otherEnvironmentInfo.environmentFilename, ".json")
		if len(differentImages) == 0 && len(currMissingImages) == 0 && len(otherMissingImages) == 0 {
			sameEnvironments.Insert(otherEnvironmentName)
			continue
		}
	}

	return sameEnvironments
}

func markdownOfCurrentEnvironmentToOthers(currEnvironmentInfo *releaseEnvironmentInfo, otherEnvironmentInfos []*releaseEnvironmentInfo, environmentToCongruentEnvironments map[string]set.Set[string]) string {
	environmentSummaryMarkdown := &strings.Builder{}
	currEnvironmentName := strings.TrimSuffix(currEnvironmentInfo.environmentFilename, ".json")
	congruentEnvironmentsForCurr := environmentToCongruentEnvironments[currEnvironmentName]
	if len(congruentEnvironmentsForCurr) == 1 {
		fmt.Fprintf(environmentSummaryMarkdown, "## %s Environment\n", strings.Join(congruentEnvironmentsForCurr.SortedList(), ", "))
	} else {
		fmt.Fprintf(environmentSummaryMarkdown, "## %s Environments\n", strings.Join(congruentEnvironmentsForCurr.SortedList(), ", "))
	}

	checkedEnvironments := set.Set[string]{}
	for _, otherEnvironmentInfo := range otherEnvironmentInfos {
		otherEnvironmentName := strings.TrimSuffix(otherEnvironmentInfo.environmentFilename, ".json")
		if congruentEnvironmentsForCurr.Has(otherEnvironmentName) {
			continue
		}
		if checkedEnvironments.Has(otherEnvironmentName) {
			continue
		}

		allDeployedImages := set.Set[string]{}
		allDeployedImages.Insert(set.KeySet(currEnvironmentInfo.pertinentInfo.deployedImages).UnsortedList()...)
		allDeployedImages.Insert(set.KeySet(otherEnvironmentInfo.pertinentInfo.deployedImages).UnsortedList()...)

		sameImages := set.Set[string]{}
		differentImageDetails := map[string]string{}
		currMissingImages := set.Set[string]{}
		otherMissingImages := set.Set[string]{}
		for _, deployedImageName := range allDeployedImages.SortedList() {
			currDeployedImageInfo := currEnvironmentInfo.pertinentInfo.deployedImages[deployedImageName]
			otherDeployedImageInfo := otherEnvironmentInfo.pertinentInfo.deployedImages[deployedImageName]
			if currDeployedImageInfo == nil {
				currMissingImages.Insert(deployedImageName)
				continue
			}
			if otherDeployedImageInfo == nil {
				otherMissingImages.Insert(deployedImageName)
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
