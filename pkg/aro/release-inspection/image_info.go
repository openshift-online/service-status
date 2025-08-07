package release_inspection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/openshift-online/service-status/pkg/apis/status"
	"k8s.io/klog/v2"
)

type ImageInfoAccessor interface {
	GetImageInfo(ctx context.Context, containerImage *status.ContainerImage) (ImageInfo, error)
}

type ThreadSafeImageInfoAccessor struct {
	pullSecretDir string

	lock sync.Mutex

	imagePullSpecToResult map[string]imageInfoResult
}

func NewThreadSafeImageInfoAccessor(pullSecretDir string) *ThreadSafeImageInfoAccessor {
	return &ThreadSafeImageInfoAccessor{
		pullSecretDir:         pullSecretDir,
		imagePullSpecToResult: make(map[string]imageInfoResult),
	}
}

func (t *ThreadSafeImageInfoAccessor) GetImageInfo(ctx context.Context, containerImage *status.ContainerImage) (ImageInfo, error) {
	imagePullSpec, err := PullSpecFromContainerImage(containerImage)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("error getting pull spec from container image: %v", err)
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	if cachedResult, ok := t.imagePullSpecToResult[imagePullSpec]; ok {
		return cachedResult.imageInfo, cachedResult.err
	}

	credentialFilePath := ""
	if credentialFilename := credentialFile(imagePullSpec); len(credentialFilename) > 0 && len(t.pullSecretDir) > 0 {
		credentialFilePath = filepath.Join(t.pullSecretDir, credentialFilename)
	}

	imageInfo, err := getImageInfoForImagePullSpec(ctx, containerImage, credentialFilePath)
	liveResult := imageInfoResult{
		imageInfo: imageInfo,
		err:       err,
	}
	t.imagePullSpecToResult[imagePullSpec] = liveResult

	return liveResult.imageInfo, liveResult.err
}

type imageInfoResult struct {
	imageInfo ImageInfo
	err       error
}

type ImageInfo struct {
	ImageCreationTime *time.Time
	SourceSHA         string
}

func PullSpecFromContainerImage(containerImage *status.ContainerImage) (string, error) {
	if containerImage == nil {
		return "", fmt.Errorf("container image is missing")
	}
	if len(containerImage.Registry) == 0 {
		return "", fmt.Errorf("container registry is missing")
	}
	if len(containerImage.Digest) == 0 {
		return "", fmt.Errorf("container digest is missing")
	}
	if len(containerImage.Repository) == 0 {
		return "", fmt.Errorf("container repository is missing")
	}
	return fmt.Sprintf("%s/%s@%s", containerImage.Registry, containerImage.Repository, containerImage.Digest), nil
}

func pullImage(ctx context.Context, imagePullSpec string, credentialFilePath string) error {
	logger := klog.FromContext(ctx)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	args := []string{"pull", imagePullSpec}
	if len(credentialFilePath) > 0 {
		args = append(args, "--authfile", credentialFilePath)
	}

	processCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	cmd := exec.CommandContext(processCtx, "podman", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start pull process: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		duration := time.Now().Sub(startTime)
		logger.Info("Failed to pull image", "imagePullSpec", imagePullSpec, "duration", duration, "args", args, "err", err)

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf(stderr.String())
		}
		return fmt.Errorf("failed to wait for pull process: %w", err)
	}

	duration := time.Now().Sub(startTime)
	logger.Info("Pulled image", "imagePullSpec", imagePullSpec, "duration", duration)
	return nil
}

func inspectImage(ctx context.Context, imagePullSpec string) (map[string]interface{}, error) {
	logger := klog.FromContext(ctx)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	args := []string{"inspect", imagePullSpec}

	processCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	startTime := time.Now()
	cmd := exec.CommandContext(processCtx, "podman", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		duration := time.Now().Sub(startTime)
		logger.Info("Failed to inspect image", "imagePullSpec", imagePullSpec, "duration", duration)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf(stderr.String())
		}
		return nil, fmt.Errorf("failed to wait for process: %w", err)
	}
	duration := time.Now().Sub(startTime)
	logger.Info("Inspected image", "imagePullSpec", imagePullSpec, "duration", duration)

	inspectResult := []map[string]interface{}{}
	if err := json.Unmarshal(stdout.Bytes(), &inspectResult); err != nil {
		return nil, fmt.Errorf("failed to unmarshal output: %w", err)
	}
	if len(inspectResult) == 0 {
		return nil, fmt.Errorf("no content")
	}
	return inspectResult[0], nil
}

func getImageInfo(imageInspect map[string]interface{}) ImageInfo {
	imageInfo := ImageInfo{}

	if c, ok := imageInspect["Config"]; ok {
		if labels, ok := c.(map[string]interface{})["Labels"]; ok {
			uncastSHA := labels.(map[string]interface{})["vcs-ref"]
			if sha, ok := uncastSHA.(string); ok {
				imageInfo.SourceSHA = sha
			}
		}
	}

	if created, ok := imageInspect["Created"]; ok {
		if createdString, ok := created.(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, createdString); err == nil {
				imageInfo.ImageCreationTime = &t
			}
		}
	}

	return imageInfo
}

func getImageInfoForImagePullSpec(ctx context.Context, containerImage *status.ContainerImage, credentialFilePath string) (ImageInfo, error) {
	pullSpec, err := PullSpecFromContainerImage(containerImage)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("error getting pull spec from container image: %v", err)
	}

	// inspect works on local data. We must pull first, then inspect
	if err := pullImage(ctx, pullSpec, credentialFilePath); err != nil {
		return ImageInfo{}, err
	}
	rawImageInfo, err := inspectImage(ctx, pullSpec)
	if err != nil {
		return ImageInfo{}, err
	}
	return getImageInfo(rawImageInfo), nil
}
