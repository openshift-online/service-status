package release_inspection

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	aro_tools_release_client "github.com/Azure/ARO-Tools/pkg/release/client"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
	"github.com/openshift-online/service-status/pkg/apis/status"
	"github.com/openshift-online/service-status/pkg/aro/sippy"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
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

type releaseAccessor struct {
	selfLookupInstance ReleaseAccessor

	numberOfDays         int
	imageInfoAccessor    ImageInfoAccessor
	componentGitAccessor ComponentsGitInfo

	releaseClientComponentIDToComponentName map[string]string
	azServiceClient                         *service.Client
}

func NewReleaseAccessor(numberOfDays int, imageInfoAccessor ImageInfoAccessor, componentGitAccessor ComponentsGitInfo) (ReleaseAccessor, error) {
	ret := &releaseAccessor{
		numberOfDays:         numberOfDays,
		imageInfoAccessor:    imageInfoAccessor,
		componentGitAccessor: componentGitAccessor,
	}

	ret.releaseClientComponentIDToComponentName = make(map[string]string)
	for _, currComponent := range HardcodedComponents {
		ret.releaseClientComponentIDToComponentName[currComponent.ReleaseClientID] = currComponent.Name
	}

	azCredential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}
	ret.azServiceClient, err = service.NewClient(aro_tools_release_client.DefaultStorageAccountURL, azCredential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create service client: %w", err)
	}

	ret.SetSelfLookupInstance(ret)
	return ret, nil
}

func (r *releaseAccessor) ListEnvironments(ctx context.Context) ([]string, error) {
	return []string{
		string(aro_tools_release_client.IntEnv),
		string(aro_tools_release_client.StgEnv),
		string(aro_tools_release_client.ProdEnv),
	}, nil
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
		otherComponent, exists := otherEnvironmentRelease.Components[component.Name]
		if !exists {
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

	releaseClient := aro_tools_release_client.NewOptions(
		aro_tools_release_client.NewFilter(
			aro_tools_release_client.WithEnvironment(aro_tools_release_client.Environment(environmentName)),
			aro_tools_release_client.WithSince(time.Now().Add(-time.Duration(r.numberOfDays)*24*time.Hour)),
			aro_tools_release_client.WithUntil(time.Now()),
		),
		aro_tools_release_client.WithServiceClient(r.azServiceClient),
		aro_tools_release_client.WithIncludeComponents(true),
	)

	// Fetch releases from blob storage
	deployments, err := releaseClient.ListReleaseDeployments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list release deployments: %w", err)
	}

	// We need to remove "empty" deployments from the slice. An empty deployment
	// is one with all its components having the same SHA as the previous deployment.
	// The first (oldest) deployment is always considered effective.
	filtered := make([]*aro_tools_release_client.ReleaseDeployment, 0, len(deployments))
	var lastComponents map[string]string
	for _, d := range deployments {
		if !reflect.DeepEqual(lastComponents, d.Components) {
			filtered = append(filtered, d)
			lastComponents = d.Components
		}
	}

	logger.Info("Found deployments from blob storage", "count", len(filtered))

	// Convert to EnvironmentRelease objects
	environmentReleases := []status.EnvironmentRelease{}
	for _, deployment := range filtered {
		envRelease, err := r.releaseDeploymentToEnvironmentRelease(ctx, deployment, r.imageInfoAccessor)
		if err != nil {
			logger.Error(err, "failed to convert deployment", "deployment", deployment.Metadata.UpstreamRevision)
			continue
		}
		environmentReleases = append(environmentReleases, *envRelease)
	}

	ciJobRuns, err := sippy.ListJobRunsForEnvironment(ctx, EnvironmentToSippyReleaseName(environmentName))
	if err != nil {
		logger.Error(err, "failed to list job runs")
	}

	ret := &status.EnvironmentReleaseList{
		TypeMeta: status.TypeMeta{
			Kind:       "EnvironmentReleaseList",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Items: []status.EnvironmentRelease{},
	}

	for i := range environmentReleases {
		envRelease := &environmentReleases[i]

		nextReleaseTime := time.Now()
		if nextReleaseIndex := i - 1; nextReleaseIndex >= 0 {
			_, nextReleaseTime, _, _ = SplitReleaseName(environmentReleases[nextReleaseIndex].ReleaseName)
		}

		if err := attachCIResultsToRelease(ctx, envRelease, nextReleaseTime, ciJobRuns); err != nil {
			logger.Error(err, "failed to attach CI results to release", "release", envRelease.ReleaseName)
		}

		ret.Items = append(ret.Items, *envRelease)
	}

	return ret, nil
}

func (r *releaseAccessor) releaseDeploymentToEnvironmentRelease(ctx context.Context, deployment *aro_tools_release_client.ReleaseDeployment, imageInfoAccessor ImageInfoAccessor) (*status.EnvironmentRelease, error) {

	releaseName := MakeReleaseName(
		deployment.Metadata.Timestamp,
		deployment.Metadata.UpstreamRevision,
	)

	environmentRelease := &status.EnvironmentRelease{
		TypeMeta: status.TypeMeta{
			Kind:       "EnvironmentRelease",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Name:                   MakeEnvironmentReleaseName(string(deployment.Target.Environment), releaseName),
		ReleaseName:            releaseName,
		SHA:                    deployment.Metadata.UpstreamRevision,
		Environment:            string(deployment.Target.Environment),
		Components:             map[string]*status.Component{},
		BlockingJobRunResults:  map[string][]status.JobRunResults{},
		InformingJobRunResults: map[string][]status.JobRunResults{},
	}

	// Convert components from digest map to Component objects
	// The ARO-Tools client returns: {"frontend": "abc123", "backend": "def456"}
	for releaseClientComponentID, digest := range deployment.Components {
		component := r.createComponentFromDigest(ctx, imageInfoAccessor, releaseClientComponentID, digest)
		if component == nil {
			continue // Skip unknown components
		}
		environmentRelease.Components[component.Name] = component
	}

	return environmentRelease, nil
}

func (r *releaseAccessor) getHardcodedComponentInfo(releaseClientComponentID string) *HardcodedComponentInfo {
	componentName, exists := r.releaseClientComponentIDToComponentName[releaseClientComponentID]
	if !exists {
		return nil
	}
	return ptr.To(HardcodedComponents[componentName])
}

func (r *releaseAccessor) createComponentFromDigest(ctx context.Context, imageInfoAccessor ImageInfoAccessor, releaseClientComponentID string, digest string) *status.Component {
	logger := klog.FromContext(ctx)

	hardcodedInfo := r.getHardcodedComponentInfo(releaseClientComponentID)
	if hardcodedInfo == nil {
		logger.Error(fmt.Errorf("no hardcoded knowledge for %q", releaseClientComponentID), "unknown release client component ID")
		return nil
	}

	component := &status.Component{
		Name: hardcodedInfo.Name,
		ImageInfo: status.ContainerImage{
			Digest:     "sha256:" + digest,
			Registry:   hardcodedInfo.ImagePullRegistry,
			Repository: hardcodedInfo.ImagePullRepository,
		},
	}

	if len(hardcodedInfo.RepositoryURL) > 0 {
		component.RepoURL = ptr.To(hardcodedInfo.RepositoryURL)
	}

	completeSourceSHAs(ctx, imageInfoAccessor, component)

	return component
}

func completeSourceSHAs(ctx context.Context, imageInfoAccessor ImageInfoAccessor, currInfo *status.Component) {
	if imageInfo, err := imageInfoAccessor.GetImageInfo(ctx, &currInfo.ImageInfo); err != nil {
		currInfo.SourceSHA = fmt.Sprintf("ERROR: %v", err)
	} else {
		currInfo.ImageCreationTime = imageInfo.ImageCreationTime
		currInfo.SourceSHA = imageInfo.SourceSHA

		switch {
		case currInfo.RepoURL != nil && strings.Contains(*currInfo.RepoURL, "github.com"):
			currInfo.PermanentURLForSourceSHA = ptr.To(*currInfo.RepoURL + "/tree/" + currInfo.SourceSHA + "/")
		case currInfo.RepoURL != nil && strings.Contains(*currInfo.RepoURL, "gitlab.cee.redhat.com"):
			currInfo.PermanentURLForSourceSHA = ptr.To(*currInfo.RepoURL + "/-/tree/" + currInfo.SourceSHA)
		}
	}
}

func attachCIResultsToRelease(
	ctx context.Context,
	envRelease *status.EnvironmentRelease,
	nextReleaseTime time.Time,
	ciJobRuns []sippy.JobRun,
) error {
	logger := klog.FromContext(ctx)

	_, currReleaseTime, _, _ := SplitReleaseName(envRelease.ReleaseName)

	for _, currJobRun := range ciJobRuns {
		jobRunTime := time.Unix(0, currJobRun.Timestamp*int64(time.Millisecond))

		// Attach jobs within the release window:
		// currReleaseTime <= jobRunTime < nextReleaseTime
		if nextReleaseTime.Before(currReleaseTime) || nextReleaseTime.Equal(currReleaseTime) {
			return fmt.Errorf("invalid release time window '%s - %s'", currReleaseTime, nextReleaseTime)
		}
		if jobRunTime.Before(currReleaseTime) || !jobRunTime.Before(nextReleaseTime) {
			continue
		}

		// Match job to hardcoded CI info
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
			logger.Info("No matching assigner found for job run", "jobRun", currJobRun.Job)
		case matchingAssigner.Category == JobImpactBlocking:
			envRelease.BlockingJobRunResults[matchingAssigner.JobVariant] = append(
				envRelease.BlockingJobRunResults[matchingAssigner.JobVariant], jobRunResult)
		case matchingAssigner.Category == JobImpactInforming:
			envRelease.InformingJobRunResults[matchingAssigner.JobVariant] = append(
				envRelease.InformingJobRunResults[matchingAssigner.JobVariant], jobRunResult)
		}
	}
	return nil
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
