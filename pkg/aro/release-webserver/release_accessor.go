package release_webserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/openshift-online/service-status/pkg/apis/status"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"github.com/openshift-online/service-status/pkg/aro/sippy"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"k8s.io/utils/set"
)

type ReleaseAccessor interface {
	ListEnvironments(ctx context.Context) ([]string, error)
	ListEnvironmentReleases(ctx context.Context) (*status.EnvironmentReleaseList, error)
	ListEnvironmentReleasesForEnvironment(ctx context.Context, environment string) (*status.EnvironmentReleaseList, error)
	GetEnvironmentRelease(ctx context.Context, environmentReleaseName string) (*status.EnvironmentRelease, error)
	GetReleaseEnvironmentDiff(ctx context.Context, environmentReleaseName string, otherEnvironmentReleaseName string) (*status.EnvironmentReleaseDiff, error)

	// this is useful to use the caching instance to delegate function calls
	SetSelfLookupInstance(ReleaseAccessor)
}

type Release struct {
	Name   string
	Commit plumbing.Hash
}

type releaseAccessor struct {
	selfLookupInstance ReleaseAccessor

	aroHCPDir            string
	numberOfDays         int
	imageInfoAccessor    release_inspection.ImageInfoAccessor
	componentGitAccessor release_inspection.ComponentsGitInfo

	gitLock              sync.Mutex
	releaseNameToInfo    map[string]*status.ReleaseDetails
	releaseNameToRelease map[string]*status.Release
}

func NewReleaseAccessor(aroHCPDir string, numberOfDays int, imageInfoAccessor release_inspection.ImageInfoAccessor, componentGitAccessor release_inspection.ComponentsGitInfo) ReleaseAccessor {
	ret := &releaseAccessor{
		aroHCPDir:            aroHCPDir,
		numberOfDays:         numberOfDays,
		imageInfoAccessor:    imageInfoAccessor,
		componentGitAccessor: componentGitAccessor,
		releaseNameToInfo:    map[string]*status.ReleaseDetails{},
		releaseNameToRelease: map[string]*status.Release{},
	}
	ret.SetSelfLookupInstance(ret)
	return ret
}

func (r *releaseAccessor) ListEnvironments(ctx context.Context) ([]string, error) {
	// TODO list the releases to locate all the available names.
	return []string{"int", "stg", "prod"}, nil
}

var interestingFiles = set.New(
	"config/config.msft.clouds-overlay.yaml",
	"config/config.yaml",
)

// listEnvironmentReleasesLookupInfo returns the environment releases from newest to oldest.
// only releases with changes are listed.
func (r *releaseAccessor) listEnvironmentReleasesLookupInfo(ctx context.Context, environmentName string) ([]*release_inspection.EnvironmentReleaseLookupInformation, error) {
	r.gitLock.Lock()
	defer r.gitLock.Unlock()

	logger := klog.FromContext(ctx)

	aroHCPRepo, err := git.PlainOpen(r.aroHCPDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open aro hcp repo: %w", err)
	}
	aroHCPHead, err := aroHCPRepo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get aro hcp head: %w", err)
	}
	aroHCPWorkTree, err := aroHCPRepo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get aro hcp worktree: %w", err)
	}
	defer func() {
		err = aroHCPWorkTree.Reset(&git.ResetOptions{
			Commit: aroHCPHead.Hash(),
			Mode:   git.HardReset,
		})
		if err != nil {
			fmt.Printf("failed to reset aro hcp worktree back to original: %v", err)
		}
	}()

	logger.Info("Working ARO HCP Head", "AROHCPHead", aroHCPHead.Hash())

	configLog, err := aroHCPRepo.Log(ptr.To(git.LogOptions{
		PathFilter: func(path string) bool {
			if strings.HasPrefix(path, "config") {
				return true
			}
			return false
		},
		Since: ptr.To(time.Now().Add(-time.Duration(r.numberOfDays) * 24 * time.Hour)),
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to get aro hcp config log: %w", err)
	}

	commitsOldestToNewest := []*object.Commit{}
	for {
		commit, err := configLog.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read aro hcp config log: %w", err)
		}
		commitsOldestToNewest = append([]*object.Commit{commit}, commitsOldestToNewest...)
	}

	logger.Info("Finding all releases.")
	environmentLookupInfoNewestToOldest := []*release_inspection.EnvironmentReleaseLookupInformation{}
	prevInterestingFiles := map[string][]byte{}
	prevEnvironmentReleaseInput := map[string][]byte{}
	for _, commit := range commitsOldestToNewest {
		firstCommitMessageLine, _, _ := strings.Cut(commit.Message, "\n")
		if commit.NumParents() != 2 && !strings.HasSuffix(firstCommitMessageLine, ")") {
			// only use commits that are due to merged PRs
			continue
		}

		err = aroHCPWorkTree.Reset(&git.ResetOptions{
			Commit: commit.Hash,
			Mode:   git.HardReset,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to reset aro hcp worktree: %w", err)
		}
		newInterestingFiles := map[string][]byte{}
		for _, filename := range interestingFiles.SortedList() {
			fileBytes, err := os.ReadFile(filepath.Join(r.aroHCPDir, filename))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("failed to read file: %w", err)
			}
			newInterestingFiles[filename] = fileBytes
		}
		if reflect.DeepEqual(prevInterestingFiles, newInterestingFiles) {
			// if no content changed, skip the commit.
			continue
		}
		prevInterestingFiles = newInterestingFiles

		// now merge the content and see if the content for a particular environment in those files changed.
		environmentReleaseInput, err := release_inspection.CompleteEnvironmentReleaseInput(ctx, r.aroHCPDir, environmentName)
		if err != nil {
			return nil, fmt.Errorf("failed to complete environment release input: %w", err)
		}
		if reflect.DeepEqual(prevEnvironmentReleaseInput, environmentReleaseInput) {
			// if no content changed, skip the commit
			continue
		}
		prevEnvironmentReleaseInput = environmentReleaseInput

		releaseName := fmt.Sprintf("%s-%s", commit.Committer.When.Format(time.RFC3339), commit.Hash.String()[:5])
		environmentLookupInfoNewestToOldest = append([]*release_inspection.EnvironmentReleaseLookupInformation{
			{
				EnvironmentName:    environmentName,
				ReleaseName:        releaseName,
				ReleaseSHA:         commit.Hash.String(),
				InterestingContent: environmentReleaseInput,
			},
		}, environmentLookupInfoNewestToOldest...)
	}
	logger.Info("Found environment releases.", "releaseCount", len(environmentLookupInfoNewestToOldest))

	return environmentLookupInfoNewestToOldest, nil
}

func (r *releaseAccessor) GetReleaseEnvironmentDiff(ctx context.Context, environmentReleaseName string, otherEnvironmentReleaseName string) (*status.EnvironmentReleaseDiff, error) {
	logger := klog.FromContext(ctx)
	logger = klog.LoggerWithValues(logger, "environmentReleaseName", environmentReleaseName)
	logger = klog.LoggerWithValues(logger, "otherEnvironmentName", otherEnvironmentReleaseName)
	ctx = klog.NewContext(ctx, logger)
	logger.Info("GetReleaseEnvironmentDiff entry")

	environmentRelease, err := r.selfLookupInstance.GetEnvironmentRelease(ctx, environmentReleaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release environment info: %w", err)
	}
	otherEnvironmentRelease, err := r.selfLookupInstance.GetEnvironmentRelease(ctx, otherEnvironmentReleaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get other release environment info: %w", err)
	}

	ret := &status.EnvironmentReleaseDiff{
		TypeMeta: status.TypeMeta{
			Kind:       "EnvironmentReleaseDiff",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Name:                        environmentReleaseName,
		OtherEnvironmentReleaseName: otherEnvironmentReleaseName,
		DifferentComponents:         map[string]*status.ComponentDiff{},
	}
	for _, component := range environmentRelease.Components {
		var otherComponent *status.Component
		for _, currOtherComponent := range otherEnvironmentRelease.Components {
			if component.Name == currOtherComponent.Name {
				otherComponent = currOtherComponent
				break
			}
		}
		if otherComponent == nil {
			continue
		}

		if component.RepoURL == nil {
			componentDiff := &status.ComponentDiff{
				Name:            component.Name,
				NumberOfChanges: -1,
				Changes: []status.ComponentChange{
					{
						ChangeType:  "Unavailable",
						Unavailable: ptr.To("No known repository link"),
					},
				},
			}
			ret.DifferentComponents[component.Name] = componentDiff
			continue
		}
		if strings.Contains(*component.RepoURL, "gitlab") {
			componentDiff := &status.ComponentDiff{
				Name:            component.Name,
				NumberOfChanges: -1,
				Changes: []status.ComponentChange{
					{
						ChangeType:  "Unavailable",
						Unavailable: ptr.To("Cannot yet reach gitlab."),
					},
				},
			}
			ret.DifferentComponents[component.Name] = componentDiff
			continue
		}
		if len(component.SourceSHA) == 0 {
			componentDiff := &status.ComponentDiff{
				Name:            component.Name,
				NumberOfChanges: -1,
				Changes: []status.ComponentChange{
					{
						ChangeType:  "Unavailable",
						Unavailable: ptr.To(fmt.Sprintf("target environment release has no SHA")),
					},
				},
			}
			ret.DifferentComponents[component.Name] = componentDiff
			continue
		}
		if len(otherComponent.SourceSHA) == 0 {
			componentDiff := &status.ComponentDiff{
				Name:            component.Name,
				NumberOfChanges: -1,
				Changes: []status.ComponentChange{
					{
						ChangeType:  "Unavailable",
						Unavailable: ptr.To(fmt.Sprintf("source environment release has no SHA")),
					},
				},
			}
			ret.DifferentComponents[component.Name] = componentDiff
			continue
		}

		gitAccessor, err := r.componentGitAccessor.GetComponentGitAccessor(ctx, component.Name)
		if err != nil {
			componentDiff := &status.ComponentDiff{
				Name:            component.Name,
				NumberOfChanges: -1,
				Changes: []status.ComponentChange{
					{
						ChangeType:  "Unavailable",
						Unavailable: ptr.To(fmt.Sprintf("failed to get component git accessor: %v", err)),
					},
				},
			}
			ret.DifferentComponents[component.Name] = componentDiff
			continue
		}
		diffs, err := gitAccessor.GetDiffForSHAs(ctx, component.SourceSHA, otherComponent.SourceSHA, 100)
		if err != nil {
			componentDiff := &status.ComponentDiff{
				Name:            component.Name,
				NumberOfChanges: -1,
				Changes: []status.ComponentChange{
					{
						ChangeType:  "Unavailable",
						Unavailable: ptr.To(fmt.Sprintf("failed to get diff component %q, curr=%q, other=%q for SHAs: %v", component.Name, component.SourceSHA, otherComponent.SourceSHA, err)),
					},
				},
			}
			ret.DifferentComponents[component.Name] = componentDiff
			continue
		}
		if len(diffs) == 0 {
			continue
		}

		componentDiff := &status.ComponentDiff{
			Name:    component.Name,
			Changes: []status.ComponentChange{},
		}
		for _, diff := range diffs {
			if len(diff.ParentHashes) < 2 {
				continue
			}
			componentDiff.NumberOfChanges++

			currChange := status.ComponentChange{
				ChangeType: "PRMerge",
				PRMerge: &status.PRMerge{
					SHA: diff.Hash.String(),
				},
			}

			// Extract PR number from merge commit message
			if prMatch := regexp.MustCompile(`Merge pull request #(\d+)`).FindStringSubmatch(diff.Message); len(prMatch) > 1 {
				if prNum, err := strconv.Atoi(prMatch[1]); err == nil {
					currChange.PRMerge.PRNumber = int32(prNum)
				}
			}

			messageLines := strings.SplitN(diff.Message, "\n", 4)
			if len(messageLines) < 3 {
				currChange.PRMerge.ChangeSummary = fmt.Sprintf("Hash: %s, Message: %s", diff.Hash.String(), messageLines[0])
			} else {
				currChange.PRMerge.ChangeSummary = messageLines[2]
			}

			componentDiff.Changes = append(componentDiff.Changes, currChange)
		}
		ret.DifferentComponents[component.Name] = componentDiff
	}

	return ret, nil
}

func (r *releaseAccessor) GetEnvironmentRelease(ctx context.Context, environmentReleaseName string) (*status.EnvironmentRelease, error) {
	logger := klog.FromContext(ctx)
	logger = klog.LoggerWithValues(logger, "environmentReleaseName", environmentReleaseName)
	ctx = klog.NewContext(ctx, logger)
	logger.Info("GetEnvironmentRelease entry")

	environmentName, releaseName, ok := SplitEnvironmentReleaseName(environmentReleaseName)
	if !ok {
		return nil, fmt.Errorf("failed to split environment release name %q", environmentReleaseName)
	}
	environmentReleases, err := r.selfLookupInstance.ListEnvironmentReleasesForEnvironment(ctx, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release info: %w", err)
	}

	for _, currEnvironmentRelease := range environmentReleases.Items {
		if currEnvironmentRelease.ReleaseName == releaseName {
			return &currEnvironmentRelease, nil
		}
	}

	return nil, fmt.Errorf("error NotFound: did not find environment release %q", releaseName)
}

func (r *releaseAccessor) ListEnvironmentReleasesForEnvironment(ctx context.Context, environmentName string) (*status.EnvironmentReleaseList, error) {
	logger := klog.FromContext(ctx)
	logger = klog.LoggerWithValues(logger, "environment", environmentName)
	ctx = klog.NewContext(ctx, logger)
	logger.Info("ListEnvironmentReleasesForEnvironment entry")

	intJobRuns, err := sippy.ListJobRunsForEnvironment(ctx, "aro-integration")
	if err != nil {
		logger.Error(err, "failed to list job runs")
	}
	stageJobRuns, err := sippy.ListJobRunsForEnvironment(ctx, "aro-stage")
	if err != nil {
		logger.Error(err, "failed to list job runs")
	}
	prodJobRuns, err := sippy.ListJobRunsForEnvironment(ctx, "aro-production")
	if err != nil {
		logger.Error(err, "failed to list job runs")
	}
	environmentNameToJobRuns := map[string][]sippy.JobRun{
		"int":  intJobRuns,
		"stg":  stageJobRuns,
		"prod": prodJobRuns,
	}
	logger.Info("gathered sippy data", "environmentNameToJobRuns", environmentNameToJobRuns)

	environmentReleasesLookupInfoNewestToOldest, err := r.listEnvironmentReleasesLookupInfo(ctx, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to list possible releases: %w", err)
	}

	ret := &status.EnvironmentReleaseList{
		TypeMeta: status.TypeMeta{
			Kind:       "EnvironmentReleaseList",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Items: []status.EnvironmentRelease{},
	}
	for _, environmentReleaseLookupInfo := range environmentReleasesLookupInfoNewestToOldest {
		releaseName := environmentReleaseLookupInfo.ReleaseName
		logger = klog.LoggerWithValues(logger, "release", releaseName)
		localCtx := klog.NewContext(ctx, logger)

		newReleaseInfo, err := release_inspection.ReleaseInfo(localCtx, r.imageInfoAccessor, environmentReleaseLookupInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to get release markdowns: %w", err)
		}

		ret.Items = append(ret.Items, *newReleaseInfo)
	}

	return ret, nil
}

func (r *releaseAccessor) ListEnvironmentReleases(ctx context.Context) (*status.EnvironmentReleaseList, error) {
	logger := klog.FromContext(ctx)
	logger.Info("ListEnvironmentReleases entry")

	environments, err := r.selfLookupInstance.ListEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	ret := &status.EnvironmentReleaseList{
		TypeMeta: status.TypeMeta{
			Kind:       "EnvironmentReleaseList",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Items: []status.EnvironmentRelease{},
	}
	for _, currEnvironment := range environments {
		currEnvironmentReleases, err := r.selfLookupInstance.ListEnvironmentReleasesForEnvironment(ctx, currEnvironment)
		if err != nil {
			return nil, fmt.Errorf("failed to list environment releases: %w", err)
		}
		ret.Items = append(ret.Items, currEnvironmentReleases.Items...)
	}

	return ret, nil
}

func (r *releaseAccessor) SetSelfLookupInstance(accessor ReleaseAccessor) {
	r.selfLookupInstance = accessor
}
