package release_inspection

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	arohcpapi "github.com/openshift-online/service-status/pkg/apis/aro-hcp"
	"k8s.io/utils/ptr"
	"k8s.io/utils/set"
)

type ReleaseEnvironmentInfo struct {
	ReleaseName         string
	ReleaseSHA          string
	EnvironmentFilename string
	configJSON          *arohcpapi.ConfigSchemaJSON
	Components          map[string]*ComponentInfo
}

// configPertinentInfo tracks the information that we want to show a diff for and summarize

type ComponentInfo struct {
	Name                 string
	ImageInfo            *arohcpapi.ContainerImage
	ImageCreationTime    *time.Time
	RepoLink             *url.URL
	SourceSHA            string
	PermLinkForSourceSHA *url.URL
}

type DeployedSourceCommits struct {
	PRURL     *url.URL
	SourceSHA string
}

func scrapeInfoForAROHCPConfig(ctx context.Context, imageInfoAccessor ImageInfoAccessor, releaseName, releaseSHA, environmentFilename string, config *arohcpapi.ConfigSchemaJSON) (*ReleaseEnvironmentInfo, error) {
	currConfigInfo := &ReleaseEnvironmentInfo{
		ReleaseName:         releaseName,
		ReleaseSHA:          releaseSHA,
		EnvironmentFilename: environmentFilename,
		configJSON:          config,
		Components:          map[string]*ComponentInfo{},
	}

	addDeployedImageForComponent := func(componentName string, containerImage *arohcpapi.ContainerImage) {
		currConfigInfo.Components[componentName] = createDeployedImageInfo(ctx,
			imageInfoAccessor,
			componentName,
			HardcodedComponents[componentName].RepositoryURL,
			containerImage,
		)
	}

	addDeployedImageForComponent("ACR Pull", &config.ACRPull.Image)
	if config.Backend != nil {
		addDeployedImageForComponent("Backend", &config.Backend.Image)
	}
	addDeployedImageForComponent("Backplane", &config.BackplaneAPI.Image)
	addDeployedImageForComponent("Cluster Service", &config.ClustersService.Image)
	addDeployedImageForComponent("Frontend", &config.Frontend.Image)
	addDeployedImageForComponent("Hypershift", config.Hypershift.Image)
	addDeployedImageForComponent("Maestro", &config.Maestro.Image)
	addDeployedImageForComponent("OcMirror", &config.ImageSync.OcMirror.Image)

	if config.Mgmt.Prometheus.PrometheusSpec != nil {
		addDeployedImageForComponent("Management Prometheus Spec", config.Mgmt.Prometheus.PrometheusSpec.Image)
	}
	if config.Svc.Prometheus != nil && config.Svc.Prometheus.PrometheusSpec != nil {
		addDeployedImageForComponent("Service Prometheus Spec", config.Svc.Prometheus.PrometheusSpec.Image)
	}

	return currConfigInfo, nil
}

func completeSourceSHAs(ctx context.Context, imageInfoAccessor ImageInfoAccessor, currInfo *ComponentInfo) {
	if imageInfo, err := imageInfoAccessor.GetImageInfo(ctx, currInfo.ImageInfo); err != nil {
		currInfo.SourceSHA = fmt.Sprintf("ERROR: %v", err)
	} else {
		currInfo.ImageCreationTime = imageInfo.ImageCreationTime
		currInfo.SourceSHA = imageInfo.SourceSHA

		switch {
		case strings.Contains(currInfo.RepoLink.String(), "github.com"):
			currInfo.PermLinkForSourceSHA = must(url.Parse(currInfo.RepoLink.String() + "/tree/" + currInfo.SourceSHA + "/"))
		case strings.Contains(currInfo.RepoLink.String(), "gitlab.cee.redhat.com"):
			currInfo.PermLinkForSourceSHA = must(url.Parse(currInfo.RepoLink.String() + "/-/tree/" + currInfo.SourceSHA))
		}
	}
}

func createDeployedImageInfo(ctx context.Context, imageInfoAccessor ImageInfoAccessor, name, repoURL string, containerImage *arohcpapi.ContainerImage) *ComponentInfo {
	repoLink := must(url.Parse(repoURL))

	deployedImageInfo := &ComponentInfo{
		Name:     name,
		RepoLink: repoLink,
	}
	if containerImage != nil {
		registry, repository, err := imagePullLocationForName(name)
		localContainerImage := *containerImage
		localContainerImage.Registry = &registry
		localContainerImage.Repository = repository
		if err != nil {
			localContainerImage.Registry = ptr.To(fmt.Sprintf("missing image pull location for %q: %v", name, err))
		}
		deployedImageInfo.ImageInfo = &localContainerImage
	}
	completeSourceSHAs(ctx, imageInfoAccessor, deployedImageInfo)

	return deployedImageInfo
}

func ChangedComponents(currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo *ReleaseEnvironmentInfo) set.Set[string] {
	changedComponents := set.Set[string]{}

	if prevReleaseEnvironmentInfo == nil {
		for _, currDeployedImageInfo := range currReleaseEnvironmentInfo.Components {
			changedComponents.Insert(currDeployedImageInfo.Name)
		}
		return changedComponents
	}

	for _, currDeployedImageInfo := range currReleaseEnvironmentInfo.Components {
		prevDeployedImageInfo := prevReleaseEnvironmentInfo.Components[currDeployedImageInfo.Name]
		if !reflect.DeepEqual(prevDeployedImageInfo.ImageInfo, currDeployedImageInfo.ImageInfo) {
			changedComponents.Insert(currDeployedImageInfo.Name)
		}
	}

	return changedComponents
}
