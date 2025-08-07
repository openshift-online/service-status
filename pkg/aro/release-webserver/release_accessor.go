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
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"k8s.io/utils/set"
)

type ReleaseAccessor interface {
	ListEnvironments(ctx context.Context) ([]string, error)
	ListReleases(ctx context.Context) (*status.ReleaseList, error)
	GetReleaseEnvironmentInfo(ctx context.Context, environmentReleaseName string) (*status.EnvironmentRelease, error)
	GetReleaseInfoForAllEnvironments(ctx context.Context, releaseName string) (*status.ReleaseDetails, error)
	GetReleaseEnvironmentDiff(ctx context.Context, environmentReleaseName string, otherEnvironmentReleaseName string) (*status.EnvironmentReleaseDiff, error)
}

type Release struct {
	Name   string
	Commit plumbing.Hash
}

type releaseAccessor struct {
	aroHCPDir            string
	numberOfDays         int
	imageInfoAccessor    release_inspection.ImageInfoAccessor
	componentGitAccessor release_inspection.ComponentsGitInfo

	gitLock              sync.Mutex
	releaseNameToInfo    map[string]*status.ReleaseDetails
	releaseNameToRelease map[string]*status.Release
}

func NewReleaseAccessor(aroHCPDir string, numberOfDays int, imageInfoAccessor release_inspection.ImageInfoAccessor, componentGitAccessor release_inspection.ComponentsGitInfo) ReleaseAccessor {
	return &releaseAccessor{
		aroHCPDir:            aroHCPDir,
		numberOfDays:         numberOfDays,
		imageInfoAccessor:    imageInfoAccessor,
		componentGitAccessor: componentGitAccessor,
		releaseNameToInfo:    map[string]*status.ReleaseDetails{},
		releaseNameToRelease: map[string]*status.Release{},
	}
}

func (r *releaseAccessor) ListEnvironments(ctx context.Context) ([]string, error) {
	// TODO list the releases to locate all the available names.
	return []string{"int", "stg", "prod"}, nil
}

var interestingFiles = set.New("config/config.msft.clouds-overlay.yaml")

func (r *releaseAccessor) getReleaseFromName(ctx context.Context, releaseName string) (*status.Release, error) {
	r.gitLock.Lock()

	release := r.releaseNameToRelease[releaseName]
	if release != nil {
		defer r.gitLock.Unlock()
		return release, nil
	}

	r.gitLock.Unlock()

	releases, err := r.ListReleases(ctx)
	if err != nil {
		for _, currRelease := range releases.Items {
			if currRelease.Name == releaseName {
				r.releaseNameToRelease[releaseName] = &currRelease
				return &currRelease, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to find release %q", releaseName)
}

func (r *releaseAccessor) ListReleases(ctx context.Context) (*status.ReleaseList, error) {
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
	releaseList := &status.ReleaseList{
		TypeMeta: status.TypeMeta{
			Kind:       "ReleaseList",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Items: nil,
	}
	prevInterestingFiles := map[string][]byte{}
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

		releaseList.Items = append([]status.Release{
			{
				TypeMeta: status.TypeMeta{
					Kind:       "Release",
					APIVersion: "service-status.hcm.openshift.io/v1",
				},
				Name: fmt.Sprintf("%s-%s", commit.Committer.When.Format(time.RFC3339), commit.Hash.String()[:5]),
				SHA:  commit.Hash.String(),
			},
		}, releaseList.Items...)
	}
	logger.Info("Found releases.", "releaseCount", len(releaseList.Items))

	for i, release := range releaseList.Items {
		r.releaseNameToRelease[release.Name] = &releaseList.Items[i]
	}

	return releaseList, nil
}

func (r *releaseAccessor) GetReleaseEnvironmentDiff(ctx context.Context, environmentReleaseName string, otherEnvironmentName string) (*status.EnvironmentReleaseDiff, error) {
	environmentRelease, err := r.GetReleaseEnvironmentInfo(ctx, environmentReleaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release environment info: %w", err)
	}
	otherEnvironmentRelease, err := r.GetReleaseEnvironmentInfo(ctx, otherEnvironmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get other release environment info: %w", err)
	}

	ret := &status.EnvironmentReleaseDiff{
		TypeMeta: status.TypeMeta{
			Kind:       "EnvironmentReleaseDiff",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Name:                        environmentReleaseName,
		OtherEnvironmentReleaseName: otherEnvironmentName,
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
			return nil, fmt.Errorf("failed to get component git accessor: %w", err)
		}
		diffs, err := gitAccessor.GetDiffForSHAs(ctx, component.SourceSHA, otherComponent.SourceSHA, 100)
		if err != nil {
			return nil, fmt.Errorf("failed to get diff component %q, curr=%q, other=%q for SHAs: %w", component.Name, component.SourceSHA, otherComponent.SourceSHA, err)
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

func (r *releaseAccessor) GetReleaseEnvironmentInfo(ctx context.Context, environmentReleaseName string) (*status.EnvironmentRelease, error) {
	environmentName, releaseName, _ := SplitEnvironmentReleaseName(environmentReleaseName)
	releaseInfo, err := r.GetReleaseInfoForAllEnvironments(ctx, releaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release info: %w", err)
	}
	return releaseInfo.Environments[environmentName], nil
}

func (r *releaseAccessor) GetReleaseInfoForAllEnvironments(ctx context.Context, releaseName string) (*status.ReleaseDetails, error) {
	enviroments, err := r.ListEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	release, err := r.getReleaseFromName(ctx, releaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	r.gitLock.Lock()
	defer r.gitLock.Unlock()

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

	localLogger := klog.FromContext(ctx)
	localLogger = klog.LoggerWithValues(localLogger, "releaseName", releaseName)
	localCtx := klog.NewContext(ctx, localLogger)

	err = aroHCPWorkTree.Reset(&git.ResetOptions{
		Commit: plumbing.NewHash(release.SHA),
		Mode:   git.HardReset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to reset aro hcp worktree: %w", err)
	}

	// if we don't have an overlay file, don't bother checking
	configOverlayFilename := filepath.Join(r.aroHCPDir, "config", "config.msft.clouds-overlay.yaml")
	if _, err := os.ReadFile(configOverlayFilename); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	releaseDiffReporter := release_inspection.NewReleaseDiffReport(r.imageInfoAccessor, releaseName, release.SHA, r.aroHCPDir, enviroments)
	newReleaseInfo, err := releaseDiffReporter.ReleaseInfoForAllEnvironments(localCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get release markdowns: %w", err)
	}

	r.releaseNameToInfo[releaseName] = newReleaseInfo

	return r.releaseNameToInfo[releaseName], nil
}
