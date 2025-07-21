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
	DeployedImages      map[string]*DeployedImageInfo
}

// configPertinentInfo tracks the information that we want to show a diff for and summarize

type DeployedImageInfo struct {
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
		DeployedImages:      map[string]*DeployedImageInfo{},
	}

	currConfigInfo.DeployedImages["Cluster Service"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"Cluster Service",
		"https://gitlab.cee.redhat.com/service/uhc-clusters-service",
		&config.ClustersService.Image,
	)
	currConfigInfo.DeployedImages["Hypershift"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"Hypershift",
		"https://github.com/openshift/hypershift",
		config.Hypershift.Image,
	)
	if config.Backend != nil {
		currConfigInfo.DeployedImages["Backend"] = createDeployedImageInfo(ctx,
			imageInfoAccessor,
			"Backend",
			"https://example.com",
			&config.Backend.Image,
		)
	}
	currConfigInfo.DeployedImages["Backplane"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"Backplane",
		"https://gitlab.cee.redhat.com/service/backplane-api",
		&config.BackplaneAPI.Image,
	)
	currConfigInfo.DeployedImages["Frontend"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"Frontend",
		"https://example.com",
		&config.Frontend.Image,
	)
	currConfigInfo.DeployedImages["OcMirror"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"OcMirror",
		"https://example.com",
		&config.ImageSync.OcMirror.Image,
	)
	// TODO
	//currConfigInfo.pertinentInfo.deployedImages["Maestro Agent Sidecar"] = createDeployedImageInfo(ctx,
	//	"Maestro Agent Sidecar",
	//	"https://example.com",
	//	&config.Maestro.Agent.Sidecar, // this isn't properly schema'd awesome
	//	prevDeployedImages)
	currConfigInfo.DeployedImages["Maestro"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"Maestro",
		"https://github.com/openshift-online/maestro/",
		&config.Maestro.Image,
	)
	// TODO
	//currConfigInfo.pertinentInfo.deployedImages["Prometheus"] = createDeployedImageInfo(ctx,
	//	"Prometheus",
	//	"https://example.com",
	//	&config.Mgmt.Prometheus.PrometheusOperator, // this isn't properly schema'd awesome
	//	prevDeployedImages)
	if config.Mgmt.Prometheus.PrometheusSpec != nil {
		currConfigInfo.DeployedImages["Management Prometheus Spec"] = createDeployedImageInfo(ctx,
			imageInfoAccessor,
			"Management Prometheus Spec",
			"https://example.com",
			config.Mgmt.Prometheus.PrometheusSpec.Image,
		)
	}
	currConfigInfo.DeployedImages["ACR Pull"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"ACR Pull",
		"https://example.com",
		&config.ACRPull.Image,
	)
	//currConfigInfo.pertinentInfo.deployedImages["Mise"] = createDeployedImageInfo(ctx,
	//	"Mise",
	//	"https://example.com",
	//	&config.Mise, // this isn't properly schema'd awesome
	//	prevDeployedImages)
	if config.Svc.Prometheus != nil && config.Svc.Prometheus.PrometheusSpec != nil {
		currConfigInfo.DeployedImages["Service Prometheus Spec"] = createDeployedImageInfo(ctx,
			imageInfoAccessor,
			"Service Prometheus Spec",
			"https://example.com",
			config.Svc.Prometheus.PrometheusSpec.Image,
		)
	}

	return currConfigInfo, nil
}

func completeSourceSHAs(ctx context.Context, imageInfoAccessor ImageInfoAccessor, currInfo *DeployedImageInfo) {
	if imageInfo, err := imageInfoAccessor.GetImageInfo(ctx, currInfo.ImageInfo); err != nil {
		currInfo.SourceSHA = fmt.Sprintf("ERROR: %v", err)
	} else {
		currInfo.ImageCreationTime = imageInfo.ImageCreationTime
		currInfo.SourceSHA = imageInfo.SourceSHA

		switch {
		case strings.Contains(currInfo.RepoLink.String(), "github.com"):
			currInfo.PermLinkForSourceSHA = must(url.Parse(currInfo.RepoLink.String() + "/tree/" + currInfo.SourceSHA + "/"))
		case strings.Contains(currInfo.RepoLink.String(), "gitlab.cee.redhat.com"):
			currInfo.PermLinkForSourceSHA = must(url.Parse(currInfo.RepoLink.String() + "/-/tree/" + currInfo.SourceSHA + "/"))
		}
	}
}

func createDeployedImageInfo(ctx context.Context, imageInfoAccessor ImageInfoAccessor, name, repoURL string, containerImage *arohcpapi.ContainerImage) *DeployedImageInfo {
	repoLink := must(url.Parse(repoURL))

	deployedImageInfo := &DeployedImageInfo{
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
		for _, currDeployedImageInfo := range currReleaseEnvironmentInfo.DeployedImages {
			changedComponents.Insert(currDeployedImageInfo.Name)
		}
		return changedComponents
	}

	for _, currDeployedImageInfo := range currReleaseEnvironmentInfo.DeployedImages {
		prevDeployedImageInfo := prevReleaseEnvironmentInfo.DeployedImages[currDeployedImageInfo.Name]
		if !reflect.DeepEqual(prevDeployedImageInfo.ImageInfo, currDeployedImageInfo.ImageInfo) {
			changedComponents.Insert(currDeployedImageInfo.Name)
		}
	}

	return changedComponents
}
