package release_webserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

type ReleaseAccessor interface {
	ListEnvironments(ctx context.Context) ([]string, error)
	ListReleases(ctx context.Context) ([]Release, error)
	GetReleaseEnvironmentInfo(ctx context.Context, release Release, environment string) (*release_inspection.ReleaseEnvironmentInfo, error)
	GetReleaseInfoForAllEnvironments(ctx context.Context, release Release) (*release_inspection.ReleaseInfo, error)
}

type Release struct {
	Name   string
	Commit object.Commit
}

type releaseAccessor struct {
	aroHCPDir         string
	imageInfoAccessor release_inspection.ImageInfoAccessor

	gitLock           sync.Mutex
	releaseNameToInfo map[string]*release_inspection.ReleaseInfo
}

func NewReleaseAccessor(aroHCPDir string, imageInfoAccessor release_inspection.ImageInfoAccessor) ReleaseAccessor {
	return &releaseAccessor{
		aroHCPDir:         aroHCPDir,
		imageInfoAccessor: imageInfoAccessor,
		releaseNameToInfo: map[string]*release_inspection.ReleaseInfo{},
	}
}

func (r releaseAccessor) ListEnvironments(ctx context.Context) ([]string, error) {
	// TODO list the releases to locate all the available names.
	return []string{"int", "stg", "prod"}, nil
}

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
		Since: ptr.To(time.Now().Add(-14 * 24 * time.Hour)),
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to get aro hcp config log: %w", err)
	}

	logger.Info("Finding all releases.")
	newestDate := time.Now().Add(48 * time.Hour)
	buildInterval := 8 * time.Hour
	releases := []Release{}
	for {
		commit, err := configLog.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read aro hcp config log: %w", err)
		}
		timeDelta := newestDate.Sub(commit.Committer.When)
		if timeDelta < buildInterval {
			continue
		}
		newestDate = commit.Committer.When
		releases = append(releases, Release{
			Name:   fmt.Sprintf("%s-%s", commit.Committer.When.Format(time.RFC3339), commit.Hash.String()[:5]),
			Commit: *commit,
		})
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
		Commit: release.Commit.Hash,
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

	releaseDiffReporter := release_inspection.NewReleaseDiffReport(r.imageInfoAccessor, release.Name, release.Commit.Hash.String(), r.aroHCPDir, enviroments)
	newReleaseInfo, err := releaseDiffReporter.ReleaseInfoForAllEnvironments(localCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get release markdowns: %w", err)
	}

	r.releaseNameToInfo[release.Name] = newReleaseInfo

	return r.releaseNameToInfo[release.Name], nil
}
