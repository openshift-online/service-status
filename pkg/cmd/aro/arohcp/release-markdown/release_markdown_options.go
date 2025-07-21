package release_markdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"github.com/openshift-online/service-status/pkg/util"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

type ReleaseMarkdownOptions struct {
	AROHCPDir string
	OutputDir string

	ImageInfoAccessor release_inspection.ImageInfoAccessor

	util.IOStreams
}

func (o *ReleaseMarkdownOptions) Run(ctx context.Context) error {
	logger := klog.FromContext(ctx)

	aroHCPRepo, err := git.PlainOpen(o.AROHCPDir)
	if err != nil {
		return fmt.Errorf("failed to open aro hcp repo: %w", err)
	}
	aroHCPHead, err := aroHCPRepo.Head()
	if err != nil {
		return fmt.Errorf("failed to get aro hcp head: %w", err)
	}
	aroHCPWorkTree, err := aroHCPRepo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get aro hcp worktree: %w", err)
	}
	defer func() {
		err = aroHCPWorkTree.Reset(&git.ResetOptions{
			Commit: aroHCPHead.Hash(),
			Mode:   git.HardReset,
		})
		if err != nil {
			fmt.Printf("failed to reset aro hcp worktree back to original: %w", err)
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
		return fmt.Errorf("failed to get aro hcp config log: %w", err)
	}

	logger.Info("Finding all releases.")
	newestDate := time.Now().Add(48 * time.Hour)
	buildInterval := 8 * time.Hour
	allReleaseCommits := []object.Commit{}
	for {
		commit, err := configLog.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read aro hcp config log: %w", err)
		}
		timeDelta := newestDate.Sub(commit.Committer.When)
		if timeDelta < buildInterval {
			continue
		}
		newestDate = commit.Committer.When
		allReleaseCommits = append(allReleaseCommits, *commit)
	}
	logger.Info("Found releases.", "releaseCount", len(allReleaseCommits))

	allReleasesInfo := &release_inspection.ReleasesInfo{}
	prevReleaseInfo := &release_inspection.ReleaseInfo{}
	for i := len(allReleaseCommits) - 1; i >= 0; i-- {
		commit := allReleaseCommits[i]
		releaseName := fmt.Sprintf("%s-%s", commit.Committer.When.Format(time.RFC3339), commit.Hash.String()[:5])

		localLogger := klog.FromContext(ctx)
		localLogger = klog.LoggerWithValues(localLogger, "releaseName", releaseName)
		localCtx := klog.NewContext(ctx, localLogger)

		err := aroHCPWorkTree.Reset(&git.ResetOptions{
			Commit: commit.Hash,
			Mode:   git.HardReset,
		})
		if err != nil {
			return fmt.Errorf("failed to reset aro hcp worktree: %w", err)
		}

		// if we don't have an overlay file, don't bother checking
		configOverlayFilename := filepath.Join(o.AROHCPDir, "config", "config.msft.clouds-overlay.yaml")
		if _, err := os.ReadFile(configOverlayFilename); errors.Is(err, os.ErrNotExist) {
			continue
		}

		releaseDiffReporter := release_inspection.NewReleaseDiffReport(o.ImageInfoAccessor, releaseName, commit.Hash.String(), o.AROHCPDir, environments)
		newReleaseInfo, err := releaseDiffReporter.ReleaseInfoForAllEnvironments(localCtx)
		if err != nil {
			return fmt.Errorf("failed to get release markdowns: %w", err)
		}

		if err := os.MkdirAll(path.Join(o.OutputDir, releaseName), 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		for _, currReleaseEnvironmentFilemame := range newReleaseInfo.GetEnvironmentFilenames() {
			currReleaseEnvironment := newReleaseInfo.GetInfoForEnvironment(currReleaseEnvironmentFilemame)
			prevReleaseEnvironmentInfo := prevReleaseInfo.GetInfoForEnvironment(currReleaseEnvironment.EnvironmentFilename)
			markdown := releaseEnvironmentMarkdown(currReleaseEnvironment, prevReleaseEnvironmentInfo)

			fullPath := filepath.Join(o.OutputDir, releaseName, currReleaseEnvironment.EnvironmentFilename+".md")
			if err := os.WriteFile(fullPath, []byte(markdown), 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", fullPath, err)
			}
		}

		prevReleaseInfo = newReleaseInfo
		allReleasesInfo.AddReleaseInfo(newReleaseInfo)

		environmentComparisonMarkdown := releaseEnvironmentSummaryMarkdown(newReleaseInfo)
		environmentComparisonPath := filepath.Join(o.OutputDir, releaseName, "environment-comparison.md")
		if err := os.WriteFile(environmentComparisonPath, []byte(environmentComparisonMarkdown), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", environmentComparisonPath, err)
		}
	}

	releaseSummaryMarkdown := allReleaseSummaryMarkdown(allReleasesInfo)
	fullPath := filepath.Join(o.OutputDir, "releases.md")
	if err := os.WriteFile(fullPath, []byte(releaseSummaryMarkdown), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", fullPath, err)
	}

	return nil
}

func stringOrErr(ret string, err error) string {
	if err != nil {
		return err.Error()
	}
	return ret
}

var environments = []string{
	"int",
	"stg",
	"prod",
}
