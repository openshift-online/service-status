package release_inspection

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	arohcpapi "github.com/openshift-online/service-status/pkg/apis/aro-hcp"
	"github.com/openshift-online/service-status/pkg/apis/status"
	"k8s.io/utils/ptr"
	"k8s.io/utils/set"
)

// configPertinentInfo tracks the information that we want to show a diff for and summarize

func getEnvironmentReleaseName(environment, release string) string {
	return fmt.Sprintf("%s---%s", environment, release)
}

func scrapeInfoForAROHCPConfig(ctx context.Context, imageInfoAccessor ImageInfoAccessor, environmentName, releaseName, releaseSHA string, config *arohcpapi.ConfigSchemaJSON) (*status.EnvironmentRelease, error) {
	currConfigInfo := &status.EnvironmentRelease{
		TypeMeta: status.TypeMeta{
			Kind:       "EnvironmentRelease",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Name:                   getEnvironmentReleaseName(environmentName, releaseName),
		ReleaseName:            releaseName,
		SHA:                    releaseSHA,
		Environment:            environmentName,
		Components:             map[string]*status.Component{},
		BlockingJobRunResults:  map[string][]status.JobRunResults{},
		InformingJobRunResults: map[string][]status.JobRunResults{},
	}

	addComponentInfo := func(componentName string, containerImage *arohcpapi.ContainerImage) {
		var digestOrSha *string
		if containerImage != nil {
			digestOrSha = ptr.To(containerImage.Digest)
		}
		currConfigInfo.Components[componentName] = createComponentInfo(ctx,
			imageInfoAccessor,
			componentName,
			HardcodedComponents[componentName].RepositoryURL,
			digestOrSha,
		)
	}

	addComponentInfoSha := func(componentName string, containerImageSha *arohcpapi.ContainerImageSha) {
		var digestOrSha *string
		if containerImageSha != nil {
			digestOrSha = ptr.To(containerImageSha.Sha)
		}
		currConfigInfo.Components[componentName] = createComponentInfo(ctx,
			imageInfoAccessor,
			componentName,
			HardcodedComponents[componentName].RepositoryURL,
			digestOrSha,
		)
	}

	if config.ACM != nil {
		addComponentInfo("ACM Operator", &config.ACM.Operator.Bundle)
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
	if config.ACM != nil {
		addComponentInfo("MCE", &config.ACM.MCE.Bundle)
	}
	addComponentInfo("OcMirror", &config.ImageSync.OcMirror.Image)
	if config.Pko != nil {
		addComponentInfo("Package Operator Package", &config.Pko.ImagePackage)
		addComponentInfo("Package Operator Manager", &config.Pko.ImageManager)
		addComponentInfo("Package Operator Remote Phase Manager", &config.Pko.RemotePhaseManager)
	}

	if config.Mgmt.Prometheus.PrometheusSpec != nil {
		addComponentInfoSha("Management Prometheus Spec", config.Mgmt.Prometheus.PrometheusSpec.Image)
	}
	if config.Svc.Prometheus != nil && config.Svc.Prometheus.PrometheusSpec != nil {
		addComponentInfoSha("Service Prometheus Spec", config.Svc.Prometheus.PrometheusSpec.Image)
	}

	return currConfigInfo, nil
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

func createComponentInfo(ctx context.Context, imageInfoAccessor ImageInfoAccessor, name, repoURL string, digestOrSha *string) *status.Component {
	componentInfo := &status.Component{
		Name: name,
	}
	if len(repoURL) > 0 {
		componentInfo.RepoURL = ptr.To(repoURL)
	}
	if digestOrSha != nil {
		registry, repository, err := imagePullLocationForName(name)
		componentInfo.ImageInfo.Digest = *digestOrSha
		componentInfo.ImageInfo.Repository = repository
		componentInfo.ImageInfo.Registry = registry
		if err != nil {
			componentInfo.ImageInfo.Registry = fmt.Sprintf("missing image pull location for %q: %v", name, err)
		}
	}
	completeSourceSHAs(ctx, imageInfoAccessor, componentInfo)

	return componentInfo
}

func ChangedComponents(currReleaseEnvironmentInfo, prevReleaseEnvironmentInfo *status.EnvironmentRelease) set.Set[string] {
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
