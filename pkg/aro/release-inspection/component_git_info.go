package release_inspection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"k8s.io/klog/v2"
)

type ComponentsGitInfo interface {
	GetComponentGitAccessor(ctx context.Context, componentName string) (ComponentGitAccessor, error)
}

type ComponentGitAccessor interface {
	GetDiffForSHAs(ctx context.Context, newerSHA, olderSHA string, topN int) ([]*object.Commit, error)
}

type componentsGitInfo struct {
	repoParentDir string

	lock              sync.Mutex
	componentGitInfos map[string]*componentGitAccessor
}

func NewComponentsGitInfo(repoParentDir string) ComponentsGitInfo {
	return &componentsGitInfo{
		repoParentDir:     repoParentDir,
		componentGitInfos: map[string]*componentGitAccessor{},
	}
}

func (c *componentsGitInfo) GetComponentGitAccessor(ctx context.Context, componentName string) (ComponentGitAccessor, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	ret, exists := c.componentGitInfos[componentName]
	if !exists {
		repoDir := filepath.Join(c.repoParentDir, strings.ReplaceAll(componentName, " ", "-"))
		ret = newComponentGitAccessor(HardcodedComponents[componentName].RepositoryURL, repoDir, HardcodedComponents[componentName].MasterBranch)
		c.componentGitInfos[componentName] = ret
	}
	return c.componentGitInfos[componentName], nil
}

type componentGitAccessor struct {
	lock sync.Mutex

	repoDir      string
	repoURL      string
	masterBranch string
}

func newComponentGitAccessor(repoURL, repoDir, masterBranch string) *componentGitAccessor {
	return &componentGitAccessor{
		repoDir:      repoDir,
		repoURL:      repoURL,
		masterBranch: masterBranch,
	}
}

func (c *componentGitAccessor) GetDiffForSHAs(ctx context.Context, newerSHA, olderSHA string, topN int) ([]*object.Commit, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	logger := klog.LoggerWithValues(klog.FromContext(ctx), "repoDir", c.repoDir, "newSHA", newerSHA, "oldSHA", olderSHA)
	ctx = klog.NewContext(ctx, logger)

	fileInfo, err := os.Stat(c.repoDir)
	switch {
	case err == nil:
		// here we need to fetch the origin main or master
		if !fileInfo.IsDir() {
			return nil, fmt.Errorf("repository path %s is not a directory", c.repoDir)
		}

		logger.Info("Fetching latest from origin", "branchName", c.masterBranch)
		componentRepo, err := git.PlainOpen(c.repoDir)
		if err != nil {
			return nil, fmt.Errorf("failed to open existing repository: %w", err)
		}
		err = componentRepo.Fetch(&git.FetchOptions{
			RemoteName: "origin",
			Progress:   os.Stdout, // TODO wire up to a logger
			RefSpecs: []config.RefSpec{
				config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/remotes/origin/%s", c.masterBranch, c.masterBranch)),
			},
		})
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil, fmt.Errorf("failed to fetch origin/main: %w", err)
		}
		worktree, err := componentRepo.Worktree()
		if err != nil {
			return nil, fmt.Errorf("failed to get worktree: %w", err)
		}
		err = worktree.Pull(&git.PullOptions{
			RemoteName:    "origin",
			ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", c.masterBranch)),
		})
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil, fmt.Errorf("failed to fast-forward main branch: %w", err)
		}

	case os.IsNotExist(err):
		logger.Info("Cloning repo", "branchName", c.masterBranch)
		// clone the repo and then close it.
		_, err := git.PlainCloneContext(ctx, c.repoDir, false, &git.CloneOptions{
			Progress: os.Stdout, // TODO wire up to a logger
			URL:      c.repoURL,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}

	case err != nil:
		return nil, fmt.Errorf("failed to get repository directory info: %w", err)
	}

	logger.Info("Getting log")
	componentRepo, err := git.PlainOpen(c.repoDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open aro hcp repo: %w", err)
	}

	newerHash, err := componentRepo.ResolveRevision(plumbing.Revision(newerSHA))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve newer SHA: %w", err)
	}
	commitLog, err := componentRepo.Log(&git.LogOptions{
		From: *newerHash,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get git log: %w", err)
	}

	var commits []*object.Commit
	reachedOlder := false
	for i := 0; i < 1000; i++ {
		commit, err := commitLog.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read git log: %w", err)
		}
		if commit.Hash.String() == olderSHA {
			reachedOlder = true
			break
		}
		commits = append(commits, commit)
	}

	if !reachedOlder {
		return nil, fmt.Errorf("older SHA %s not found in commit history of main branch within 1000 commits", olderSHA)
	}

	return commits, nil
}

type dummyComponentsGitInfo struct {
}

func NewDummyComponentsGitInfo() ComponentsGitInfo {
	return &dummyComponentsGitInfo{}
}

func (c *dummyComponentsGitInfo) GetComponentGitAccessor(ctx context.Context, componentName string) (ComponentGitAccessor, error) {
	return &dummyComponentGitAccessor{}, nil
}

type dummyComponentGitAccessor struct {
}

func (c *dummyComponentGitAccessor) GetDiffForSHAs(ctx context.Context, newerSHA, olderSHA string, topN int) ([]*object.Commit, error) {
	return nil, nil
}
