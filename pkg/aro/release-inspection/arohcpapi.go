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

	addComponentInfo := func(componentName string, containerImage *arohcpapi.ContainerImage) {
		currConfigInfo.Components[componentName] = createComponentInfo(ctx,
			imageInfoAccessor,
			componentName,
			HardcodedComponents[componentName].RepositoryURL,
			containerImage,
		)
	}

	addComponentInfo("ACR Pull", &config.ACRPull.Image)
	if config.Backend != nil {
		addComponentInfo("Backend", &config.Backend.Image)
	}
	addComponentInfo("Backplane", &config.BackplaneAPI.Image)
	addComponentInfo("Cluster Service", &config.ClustersService.Image)
	addComponentInfo("Frontend", &config.Frontend.Image)
	addComponentInfo("Hypershift", config.Hypershift.Image)
	addComponentInfo("Maestro", &config.Maestro.Image)
	addComponentInfo("OcMirror", &config.ImageSync.OcMirror.Image)

	if config.Mgmt.Prometheus.PrometheusSpec != nil {
		addComponentInfo("Management Prometheus Spec", config.Mgmt.Prometheus.PrometheusSpec.Image)
	}
	if config.Svc.Prometheus != nil && config.Svc.Prometheus.PrometheusSpec != nil {
		addComponentInfo("Service Prometheus Spec", config.Svc.Prometheus.PrometheusSpec.Image)
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

func createComponentInfo(ctx context.Context, imageInfoAccessor ImageInfoAccessor, name, repoURL string, containerImage *arohcpapi.ContainerImage) *ComponentInfo {
	repoLink := must(url.Parse(repoURL))

	componentInfo := &ComponentInfo{
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
		componentInfo.ImageInfo = &localContainerImage
	}
	completeSourceSHAs(ctx, imageInfoAccessor, componentInfo)

	return componentInfo
}

func ChangedComponents(currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo *ReleaseEnvironmentInfo) set.Set[string] {
	changedComponents := set.Set[string]{}

	if prevReleaseEnvironmentInfo == nil {
		for _, currComponent := range currReleaseEnvironmentInfo.Components {
			changedComponents.Insert(currComponent.Name)
		}
		return changedComponents
	}

	for _, currComponent := range currReleaseEnvironmentInfo.Components {
		prevComponent := prevReleaseEnvironmentInfo.Components[currComponent.Name]
		if !reflect.DeepEqual(prevComponent.ImageInfo, currComponent.ImageInfo) {
			changedComponents.Insert(currComponent.Name)
		}
	}

	return changedComponents
}
