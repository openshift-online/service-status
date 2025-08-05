package release_webserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"k8s.io/utils/set"
)

type ReleaseAccessor interface {
	ListEnvironments(ctx context.Context) ([]string, error)
	ListReleases(ctx context.Context) ([]Release, error)
	GetReleaseEnvironmentInfo(ctx context.Context, release Release, environment string) (*release_inspection.ReleaseEnvironmentInfo, error)
	GetReleaseInfoForAllEnvironments(ctx context.Context, release Release) (*release_inspection.ReleaseInfo, error)
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

	gitLock           sync.Mutex
	releaseNameToInfo map[string]*release_inspection.ReleaseInfo
}

func NewReleaseAccessor(aroHCPDir string, numberOfDays int, imageInfoAccessor release_inspection.ImageInfoAccessor, componentGitAccessor release_inspection.ComponentsGitInfo) ReleaseAccessor {
	return &releaseAccessor{
		aroHCPDir:            aroHCPDir,
		numberOfDays:         numberOfDays,
		imageInfoAccessor:    imageInfoAccessor,
		componentGitAccessor: componentGitAccessor,
		releaseNameToInfo:    map[string]*release_inspection.ReleaseInfo{},
	}
}

func (r releaseAccessor) ListEnvironments(ctx context.Context) ([]string, error) {
	// TODO list the releases to locate all the available names.
	return []string{"int", "stg", "prod"}, nil
}

var interestingFiles = set.New("config/config.msft.clouds-overlay.yaml")

func (r releaseAccessor) ListReleases(ctx context.Context) ([]Release, error) {
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
	releases := []Release{}
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

		releases = append([]Release{{
			Name:   fmt.Sprintf("%s-%s", commit.Committer.When.Format(time.RFC3339), commit.Hash.String()[:5]),
			Commit: commit.Hash,
		}}, releases...)
	}
	logger.Info("Found releases.", "releaseCount", len(releases))

	return releases, nil
}

func (r *releaseAccessor) GetReleaseEnvironmentInfo(ctx context.Context, release Release, environment string) (*release_inspection.ReleaseEnvironmentInfo, error) {
	releaseInfo, err := r.GetReleaseInfoForAllEnvironments(ctx, release)
	if err != nil {
		return nil, fmt.Errorf("failed to get release info: %w", err)
	}
	return releaseInfo.GetInfoForEnvironment(environment), nil
}

func (r *releaseAccessor) GetReleaseInfoForAllEnvironments(ctx context.Context, release Release) (*release_inspection.ReleaseInfo, error) {
	enviroments, err := r.ListEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
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
	localLogger = klog.LoggerWithValues(localLogger, "releaseName", release.Name)
	localCtx := klog.NewContext(ctx, localLogger)

	err = aroHCPWorkTree.Reset(&git.ResetOptions{
		Commit: release.Commit,
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

	releaseDiffReporter := release_inspection.NewReleaseDiffReport(r.imageInfoAccessor, release.Name, release.Commit.String(), r.aroHCPDir, enviroments)
	newReleaseInfo, err := releaseDiffReporter.ReleaseInfoForAllEnvironments(localCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get release markdowns: %w", err)
	}

	r.releaseNameToInfo[release.Name] = newReleaseInfo

	return r.releaseNameToInfo[release.Name], nil
}
