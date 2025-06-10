package release_markdown

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/dustin/go-humanize"
	arohcpapi "github.com/openshift-online/service-status/pkg/apis/aro-hcp"
	"k8s.io/klog/v2"
	"k8s.io/utils/set"
)

type releaseDiffReport struct {
	// these fields are for input, not output

	imageInfoAccessor ImageInfoAccessor
	releaseName       string
	environments      []string
	repoDir           string
	prevReleaseInfo   *releaseInfo
}

func newReleaseDiffReport(imageInfoAccessor ImageInfoAccessor, releaseName string, repoDir string, environments []string, prevReleaseInfo *releaseInfo) *releaseDiffReport {
	return &releaseDiffReport{
		imageInfoAccessor: imageInfoAccessor,
		releaseName:       releaseName,
		repoDir:           repoDir,
		environments:      environments,
		prevReleaseInfo:   prevReleaseInfo,
	}
}

func (r *releaseDiffReport) releaseInfoForAllEnvironments(ctx context.Context) (*releaseInfo, error) {
	ret := &releaseInfo{
		releaseName: r.releaseName,
	}

	for _, environmentFilename := range r.environments {
		localLogger := klog.FromContext(ctx)
		localLogger = klog.LoggerWithValues(localLogger, "configFile", environmentFilename)
		localCtx := klog.NewContext(ctx, localLogger)

		fullPath := filepath.Join(r.repoDir, "config", environmentFilename)
		jsonBytes, err := os.ReadFile(fullPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", fullPath, err)
		}

		prevReleaseEnvironmentInfo := r.prevReleaseInfo.getInfoForEnvironment(environmentFilename)
		currReleaseEnvironmentInfo, err := r.releaseMarkdownForConfigJSON(localCtx, environmentFilename, jsonBytes, prevReleaseEnvironmentInfo)
		if err != nil {
			// the schema in ARO-HCP is changing incompatibly, so we are not guaranteed to be able to parse older releases
			localLogger.Error(err, "failed to release markdown for config JSON.  Continuing...")
			continue
			//return nil, fmt.Errorf("failed to create markdown for %s: %w", fullPath, err)
		}
		ret.addEnvironment(currReleaseEnvironmentInfo)
	}

	return ret, nil
}

func (r *releaseDiffReport) releaseMarkdownForConfigJSON(ctx context.Context, environmentName string, currReleaseEnvironmentJSON []byte, prevReleaseEnvironmentInfo *releaseEnvironmentInfo) (*releaseEnvironmentInfo, error) {
	config := &arohcpapi.ConfigSchemaJSON{}
	err := json.Unmarshal(currReleaseEnvironmentJSON, config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	ret, err := r.releaseMarkdownForConfig(ctx, environmentName, config, prevReleaseEnvironmentInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to create markdown for %s: %w", r.releaseName, err)
	}
	return ret, nil
}

func (r *releaseDiffReport) releaseMarkdownForConfig(ctx context.Context, environmentName string, config *arohcpapi.ConfigSchemaJSON, prevReleaseEnvironmentInfo *releaseEnvironmentInfo) (*releaseEnvironmentInfo, error) {
	logger := klog.FromContext(ctx)
	logger.Info("Scraping info")

	currConfigInfo, err := scrapeInfoForAROHCPConfig(ctx, r.imageInfoAccessor, r.releaseName, environmentName, config, prevReleaseEnvironmentInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to create markdown for %s: %w", r.releaseName, err)
	}

	return currConfigInfo, nil
}

func markdownForPertinentInfo(info *DeployedImageInfo) string {
	markdown := &strings.Builder{}
	fmt.Fprintf(markdown, "### [%s](%s)\n", info.Name, info.RepoLink)
	fmt.Fprintf(markdown, "* %s\n", info.RepoLink)
	fmt.Fprintf(markdown, "* Pull Spec: %s\n", stringOrErr(pullSpecFromContainerImage(info.ImageInfo)))
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
	fmt.Fprintf(markdown, "* Changes: (%d changes)\n", info.CountOfCommitsSincePreviousSHA)
	fmt.Fprintf(markdown, "\n")

	return markdown.String()
}

func releaseEnvironmentMarkdown(currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo *releaseEnvironmentInfo) string {
	markdown := &strings.Builder{}
	fmt.Fprintf(markdown, "# Release %v\n\n", currReleaseEnvironmentInfo.releaseName)

	fmt.Fprintf(markdown, "## Diff\n\n")
	if len(currReleaseEnvironmentInfo.changedComponents) == 0 {
		fmt.Fprintf(markdown, "*No Changes*\n\n")
	} else {
		for _, componentName := range set.KeySet(currReleaseEnvironmentInfo.pertinentInfo.deployedImages).SortedList() {
			currInfo := currReleaseEnvironmentInfo.pertinentInfo.deployedImages[componentName]
			if prevReleaseEnvironmentInfo != nil {
				prevInfo := prevReleaseEnvironmentInfo.pertinentInfo.deployedImages[componentName]
				if reflect.DeepEqual(currInfo, prevInfo) {
					continue
				}
			}
			fmt.Fprintf(markdown, markdownForPertinentInfo(currInfo))
		}

	}

	fmt.Fprintf(markdown, "## Content\n\n")
	for _, componentName := range set.KeySet(currReleaseEnvironmentInfo.pertinentInfo.deployedImages).SortedList() {
		info := currReleaseEnvironmentInfo.pertinentInfo.deployedImages[componentName]
		fmt.Fprintf(markdown, markdownForPertinentInfo(info))
	}

	return markdown.String()
}

func allReleaseSummaryMarkdown(allReleasesInfo *releasesInfo) string {
	releaseSummaryMarkdown := &strings.Builder{}

	for _, environmentFilename := range allReleasesInfo.getEnvironmentFilenames() {
		fmt.Fprintf(releaseSummaryMarkdown, "# %s Releases\n\n", strings.TrimSuffix(environmentFilename, ".json"))

		for _, currReleaseName := range allReleasesInfo.getReleaseNames() {
			currReleaseInfo := allReleasesInfo.getReleaseInfo(currReleaseName)
			currReleaseEnvironmentInfo := currReleaseInfo.getInfoForEnvironment(environmentFilename)
			if currReleaseEnvironmentInfo == nil {
				continue
			}

			// TODO table
			fmt.Fprintf(releaseSummaryMarkdown, "* [%s](%s) %d changes (%v)\n",
				currReleaseEnvironmentInfo.releaseName,
				"TODO",
				len(currReleaseEnvironmentInfo.changedComponents),
				strings.Join(currReleaseEnvironmentInfo.changedComponents.SortedList(), ", "),
			)
		}
		fmt.Fprintf(releaseSummaryMarkdown, "\n")
	}

	return releaseSummaryMarkdown.String()
}
