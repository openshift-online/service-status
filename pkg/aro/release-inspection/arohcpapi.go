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
	EnvironmentFilename string
	ChangedComponents   set.Set[string]
	configJSON          *arohcpapi.ConfigSchemaJSON
	DeployedImages      map[string]*DeployedImageInfo
}

// configPertinentInfo tracks the information that we want to show a diff for and summarize

type DeployedImageInfo struct {
	Name                           string
	ImageInfo                      *arohcpapi.ContainerImage
	ImageCreationTime              *time.Time
	RepoLink                       *url.URL
	SourceSHA                      string
	PermLinkForSourceSHA           *url.URL
	PreviousSourceSHA              string
	CountOfCommitsSincePreviousSHA int32
	CommitsSincePreviousSHA        []DeployedSourceCommits
}

type DeployedSourceCommits struct {
	PRURL     *url.URL
	SourceSHA string
}

func scrapeInfoForAROHCPConfig(ctx context.Context, imageInfoAccessor ImageInfoAccessor, releaseName, environmentFilename string, config *arohcpapi.ConfigSchemaJSON, prevConfigInfo *ReleaseEnvironmentInfo) (*ReleaseEnvironmentInfo, error) {
	currConfigInfo := &ReleaseEnvironmentInfo{
		ReleaseName:         releaseName,
		EnvironmentFilename: environmentFilename,
		configJSON:          config,
		ChangedComponents:   make(set.Set[string]),
		DeployedImages:      map[string]*DeployedImageInfo{},
	}

	prevDeployedImages := map[string]*DeployedImageInfo{}
	if prevConfigInfo != nil {
		prevDeployedImages = prevConfigInfo.DeployedImages
	}

	currConfigInfo.DeployedImages["Cluster Service"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"Cluster Service",
		"https://gitlab.cee.redhat.com/service/uhc-clusters-service",
		&config.ClustersService.Image,
		prevDeployedImages)
	currConfigInfo.DeployedImages["Hypershift"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"Hypershift",
		"https://github.com/openshift/hypershift",
		config.Hypershift.Image,
		prevDeployedImages)
	if config.Backend != nil {
		currConfigInfo.DeployedImages["Backend"] = createDeployedImageInfo(ctx,
			imageInfoAccessor,
			"Backend",
			"https://example.com",
			&config.Backend.Image,
			prevDeployedImages)
	}
	currConfigInfo.DeployedImages["Backplane"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"Backplane",
		"https://gitlab.cee.redhat.com/service/backplane-api",
		&config.BackplaneAPI.Image,
		prevDeployedImages)
	currConfigInfo.DeployedImages["Frontend"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"Frontend",
		"https://example.com",
		&config.Frontend.Image,
		prevDeployedImages)
	currConfigInfo.DeployedImages["OcMirror"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"OcMirror",
		"https://example.com",
		&config.ImageSync.OcMirror.Image,
		prevDeployedImages)
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
		prevDeployedImages)
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
			prevDeployedImages)
	}
	currConfigInfo.DeployedImages["ACR Pull"] = createDeployedImageInfo(ctx,
		imageInfoAccessor,
		"ACR Pull",
		"https://example.com",
		&config.ACRPull.Image,
		prevDeployedImages)
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
			prevDeployedImages)
	}

	if prevConfigInfo == nil {
		currConfigInfo.ChangedComponents = set.KeySet(currConfigInfo.DeployedImages)
	} else {
		for _, currDeployedImageInfo := range currConfigInfo.DeployedImages {
			prevDeployedImageInfo := prevConfigInfo.DeployedImages[currDeployedImageInfo.Name]
			if !reflect.DeepEqual(prevDeployedImageInfo.ImageInfo, currDeployedImageInfo.ImageInfo) {
				currConfigInfo.ChangedComponents.Insert(currDeployedImageInfo.Name)
			}
		}
	}

	return currConfigInfo, nil
}

func completeSourceSHAs(ctx context.Context, imageInfoAccessor ImageInfoAccessor, currInfo, prevInfo *DeployedImageInfo) {
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

	if prevInfo == nil {
		return
	}

	if imageInfo, err := imageInfoAccessor.GetImageInfo(ctx, prevInfo.ImageInfo); err != nil {
		currInfo.PreviousSourceSHA = fmt.Sprintf("ERROR: %v", err)
	} else {
		currInfo.PreviousSourceSHA = imageInfo.SourceSHA
	}
}

func createDeployedImageInfo(ctx context.Context, imageInfoAccessor ImageInfoAccessor, name, repoURL string, containerImage *arohcpapi.ContainerImage, prevDeployedImages map[string]*DeployedImageInfo) *DeployedImageInfo {
	repoLink := must(url.Parse(repoURL))

	deployedImageInfo := &DeployedImageInfo{
		Name:                           name,
		RepoLink:                       repoLink,
		PreviousSourceSHA:              "",
		CountOfCommitsSincePreviousSHA: 0,
		CommitsSincePreviousSHA:        nil,
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
	prevClusterServiceInfo := prevDeployedImages[deployedImageInfo.Name]
	completeSourceSHAs(ctx, imageInfoAccessor, deployedImageInfo, prevClusterServiceInfo)

	return deployedImageInfo
}
