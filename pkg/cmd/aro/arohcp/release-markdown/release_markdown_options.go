package release_markdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/openshift-online/service-status/pkg/util"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"k8s.io/utils/set"
)

type ReleaseMarkdownOptions struct {
	AROHCPDir string
	OutputDir string

	ImageInfoAccessor ImageInfoAccessor

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

	allReleasesInfo := &releasesInfo{}
	prevReleaseInfo := &releaseInfo{}
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

		releaseDiffReporter := newReleaseDiffReport(o.ImageInfoAccessor, releaseName, o.AROHCPDir, environments, prevReleaseInfo)
		newReleaseInfo, err := releaseDiffReporter.releaseInfoForAllEnvironments(localCtx)
		if err != nil {
			return fmt.Errorf("failed to get release markdowns: %w", err)
		}

		if err := os.MkdirAll(path.Join(o.OutputDir, releaseName), 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		for _, currReleaseEnvironment := range newReleaseInfo.environmentToEnvironmentInfo {
			prevReleaseEnvironmentInfo := prevReleaseInfo.getInfoForEnvironment(currReleaseEnvironment.environmentFilename)
			markdown := releaseEnvironmentMarkdown(currReleaseEnvironment, prevReleaseEnvironmentInfo)

			fullPath := filepath.Join(o.OutputDir, releaseName, currReleaseEnvironment.environmentFilename+".md")
			if err := os.WriteFile(fullPath, []byte(markdown), 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", fullPath, err)
			}
		}

		prevReleaseInfo = newReleaseInfo
		allReleasesInfo.addReleaseInfo(newReleaseInfo)

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

type releasesInfo struct {
	releaseToInfo map[string]*releaseInfo
}

func (r *releasesInfo) getReleaseNames() []string {
	if r == nil {
		return nil
	}

	releasesOldestFirst := set.KeySet(r.releaseToInfo).SortedList()
	sort.Sort(sort.Reverse(sort.StringSlice(releasesOldestFirst)))
	return releasesOldestFirst
}

func (r *releasesInfo) getEnvironmentFilenames() []string {
	if r == nil {
		return nil
	}
	environmentNames := set.Set[string]{}
	for _, currReleaseInfo := range r.releaseToInfo {
		environmentNames.Insert(set.KeySet(currReleaseInfo.environmentToEnvironmentInfo).UnsortedList()...)
	}
	return environmentNames.SortedList()
}

func (r *releasesInfo) addReleaseInfo(newReleaseInfo *releaseInfo) {
	if r.releaseToInfo == nil {
		r.releaseToInfo = make(map[string]*releaseInfo)
	}
	r.releaseToInfo[newReleaseInfo.releaseName] = newReleaseInfo
}

func (r *releasesInfo) getReleaseInfo(release string) *releaseInfo {
	if r == nil {
		return nil
	}
	return r.releaseToInfo[release]
}

type releaseInfo struct {
	releaseName                  string
	environmentToEnvironmentInfo map[string]*releaseEnvironmentInfo
}

func (r *releaseInfo) getEnvironmentFilenames() []string {
	if r == nil {
		return nil
	}
	environmentNames := set.KeySet(r.environmentToEnvironmentInfo)
	return environmentNames.SortedList()
}

func (r *releaseInfo) addEnvironment(environmentInfo *releaseEnvironmentInfo) {
	if r.environmentToEnvironmentInfo == nil {
		r.environmentToEnvironmentInfo = make(map[string]*releaseEnvironmentInfo)
	}
	r.environmentToEnvironmentInfo[environmentInfo.environmentFilename] = environmentInfo
}

func (r *releaseInfo) getInfoForEnvironment(environment string) *releaseEnvironmentInfo {
	if r == nil {
		return nil
	}
	return r.environmentToEnvironmentInfo[environment]
}

type DeployedSourceCommits struct {
	PRURL     *url.URL
	SourceSHA string
}

func must[T any](ret T, err error) T {
	if err != nil {
		panic(err)
	}
	return ret
}

func stringOrErr(ret string, err error) string {
	if err != nil {
		return err.Error()
	}
	return ret
}

var environments = []string{
	"public-cloud-dev.json",
	"public-cloud-msft-int.json",
	"public-cloud-msft-stg.json",
	"public-cloud-ntly.json",
	"public-cloud-pers.json",
}
