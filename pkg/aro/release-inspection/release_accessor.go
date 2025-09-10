package release_inspection

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
	imageInfoAccessor    ImageInfoAccessor
	componentGitAccessor ComponentsGitInfo

	gitLock              sync.Mutex
	releaseNameToInfo    map[string]*status.ReleaseDetails
	releaseNameToRelease map[string]*status.Release
}

func NewReleaseAccessor(aroHCPDir string, numberOfDays int, imageInfoAccessor ImageInfoAccessor, componentGitAccessor ComponentsGitInfo) ReleaseAccessor {
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
func (r *releaseAccessor) listEnvironmentReleasesLookupInfo(ctx context.Context, environmentName string) ([]*EnvironmentReleaseLookupInformation, error) {
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
	environmentLookupInfoNewestToOldest := []*EnvironmentReleaseLookupInformation{}
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
		environmentReleaseInput, err := CompleteEnvironmentReleaseInput(ctx, r.aroHCPDir, environmentName)
		if err != nil {
			return nil, fmt.Errorf("failed to complete environment release input: %w", err)
		}
		if reflect.DeepEqual(prevEnvironmentReleaseInput, environmentReleaseInput) {
			// if no content changed, skip the commit
			continue
		}
		prevEnvironmentReleaseInput = environmentReleaseInput

		releaseName := MakeReleaseNameFromCommit(*commit)
		environmentLookupInfoNewestToOldest = append([]*EnvironmentReleaseLookupInformation{
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

			switch {
			case strings.Contains(ptr.Deref(component.RepoURL, ""), "github.com"):
				currChange := status.ComponentChange{
					ChangeType: "GithubPRMerge",
					GithubPRMerge: &status.GithubPRMerge{
						SHA: diff.Hash.String(),
					},
				}

				// Extract PR number from merge commit message
				if prMatch := regexp.MustCompile(`Merge pull request #(\d+)`).FindStringSubmatch(diff.Message); len(prMatch) > 1 {
					if prNum, err := strconv.Atoi(prMatch[1]); err == nil {
						currChange.GithubPRMerge.PRNumber = int32(prNum)
					}
				}

				messageLines := strings.SplitN(diff.Message, "\n", 4)
				if len(messageLines) < 3 {
					currChange.GithubPRMerge.ChangeSummary = fmt.Sprintf("Hash: %s, Message: %s", diff.Hash.String(), messageLines[0])
				} else {
					currChange.GithubPRMerge.ChangeSummary = messageLines[2]
				}

				componentDiff.Changes = append(componentDiff.Changes, currChange)

			case strings.Contains(ptr.Deref(component.RepoURL, ""), "gitlab.cee.redhat.com"):
				currChange := status.ComponentChange{
					ChangeType: "GitlabMRMerge",
					GitlabMRMerge: &status.GitlabMRMerge{
						SHA: diff.Hash.String(),
					},
				}

				// Extract MR number from merge commit message
				if mrMatch := regexp.MustCompile(`See merge request .*!(\d+)`).FindStringSubmatch(diff.Message); len(mrMatch) > 1 {
					if mrNum, err := strconv.Atoi(mrMatch[1]); err == nil {
						currChange.GitlabMRMerge.MRNumber = int32(mrNum)
					}
				}

				messageLines := strings.SplitN(diff.Message, "\n", 4)
				if len(messageLines) < 3 {
					currChange.GitlabMRMerge.ChangeSummary = fmt.Sprintf("Hash: %s, Message: %s", diff.Hash.String(), messageLines[0])
				} else {
					currChange.GitlabMRMerge.ChangeSummary = messageLines[2]
				}

				componentDiff.Changes = append(componentDiff.Changes, currChange)

			default:
				componentDiff.Changes = append(componentDiff.Changes, status.ComponentChange{
					ChangeType:  "Unavailable",
					Unavailable: ptr.To(fmt.Sprintf("failed to understand change git accessor: %v", err)),
				})
			}
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

	ciJobRuns, err := sippy.ListJobRunsForEnvironment(ctx, EnvironmentToSippyReleaseName(environmentName))
	if err != nil {
		logger.Error(err, "failed to list job runs")
	}

	environmentReleasesLookupInfoNewestToOldest, err := r.listEnvironmentReleasesLookupInfo(ctx, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to list possible releases: %w", err)
	}

	partialEnvironmentReleases := []status.EnvironmentRelease{}
	for i, environmentReleaseLookupInfo := range environmentReleasesLookupInfoNewestToOldest {
		releaseName := environmentReleaseLookupInfo.ReleaseName
		loopLogger := klog.LoggerWithValues(logger, "lookupEnvironment", environmentReleaseLookupInfo.EnvironmentName, "lookupReleaseName", releaseName, "i", i, "len", len(environmentReleasesLookupInfoNewestToOldest))
		localCtx := klog.NewContext(ctx, loopLogger)
		loopLogger.Info("starting execution")

		newReleaseInfo, err := ReleaseInfo(localCtx, r.imageInfoAccessor, environmentReleaseLookupInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to get release markdowns: %w", err)
		}

		if len(partialEnvironmentReleases) > 0 {
			moreRecentPartialEnvironmentRelease := partialEnvironmentReleases[len(partialEnvironmentReleases)-1]
			changedComponents := ChangedComponents(newReleaseInfo, &moreRecentPartialEnvironmentRelease)
			if len(changedComponents) == 0 {
				// if nothing changed, then the more recent release isn't actually a new release, so replace it with this current one
				partialEnvironmentReleases[len(partialEnvironmentReleases)-1] = *newReleaseInfo
				continue
			}
		}

		partialEnvironmentReleases = append(partialEnvironmentReleases, *newReleaseInfo)
	}

	ret := &status.EnvironmentReleaseList{
		TypeMeta: status.TypeMeta{
			Kind:       "EnvironmentReleaseList",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Items: []status.EnvironmentRelease{},
	}
	// now add the CI status to these releases
	for i, newReleaseInfo := range partialEnvironmentReleases {
		loopLogger := klog.LoggerWithValues(logger, "newEnvironmentReleaseName", newReleaseInfo.Name)
		loopLogger.Info("starting execution")

		nextReleaseTime := time.Now()
		if nextReleaseIndex := i - 1; nextReleaseIndex >= 0 {
			_, nextReleaseTime, _, _ = SplitReleaseName(environmentReleasesLookupInfoNewestToOldest[nextReleaseIndex].ReleaseName)
		}
		_, newReleaseTime, _, _ := SplitReleaseName(newReleaseInfo.ReleaseName)

		// find all job runs between the newReleaseTime and the nextReleaseTime
		for _, currJobRun := range ciJobRuns {
			jobRunTime := time.Unix(0, currJobRun.Timestamp*int64(time.Millisecond))
			if jobRunTime.After(nextReleaseTime) {
			}
			if jobRunTime.After(nextReleaseTime) || jobRunTime.Before(newReleaseTime) {
				continue
			}

			var matchingAssigner *HardcodedCIInfo
			for _, ciAssigner := range HardcodedCIInfos {
				for _, currRegex := range ciAssigner.JobRegexes {
					if currRegex.MatchString(currJobRun.Job) {
						matchingAssigner = &ciAssigner
						break
					}
				}
				if matchingAssigner != nil {
					break
				}
			}

			jobRunResult := status.JobRunResults{
				JobName:       currJobRun.Job,
				OverallResult: status.JobOverallResult(currJobRun.OverallResult),
				URL:           currJobRun.URL,
			}

			switch {
			case matchingAssigner == nil:
				loopLogger.Info("No matching assigner found for job run", "jobRun", currJobRun.Job)
			case matchingAssigner.Category == JobImpactBlocking:
				newReleaseInfo.BlockingJobRunResults[matchingAssigner.JobVariant] = append(newReleaseInfo.BlockingJobRunResults[matchingAssigner.JobVariant], jobRunResult)
			case matchingAssigner.Category == JobImpactInforming:
				newReleaseInfo.InformingJobRunResults[matchingAssigner.JobVariant] = append(newReleaseInfo.InformingJobRunResults[matchingAssigner.JobVariant], jobRunResult)
			}
		}

		ret.Items = append(ret.Items, newReleaseInfo)
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
