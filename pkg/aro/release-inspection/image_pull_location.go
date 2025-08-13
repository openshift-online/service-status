package release_inspection

import (
	"fmt"
	"strings"
	"time"
)

type HardcodedComponentInfo struct {
	Name                string
	ImagePullRegistry   string
	ImagePullRepository string
	RepositoryURL       string
	MasterBranch        string

	// how old can an image be before we say there is a need to update it
	LatencyThreshold time.Duration
}

var (
	teamLatency    = 2 * 24 * time.Hour
	orgLatency     = 3 * 24 * time.Hour
	companyLatency = 4 * 24 * time.Hour
	worldLatency   = 60 * 24 * time.Hour
)

var HardcodedComponents = map[string]HardcodedComponentInfo{
	"ACM Operator": {
		Name:                "ACM Operator",
		ImagePullRegistry:   "arohcpsvcdev.azurecr.io",
		ImagePullRepository: "rhacm2/acm-operator-bundle",
		RepositoryURL:       "https://github.com/stolostron/acm-operator-bundle",
		MasterBranch:        "main",
	},
	"ACR Pull": {
		Name:                "ACR Pull",
		ImagePullRegistry:   "mcr.microsoft.com",
		ImagePullRepository: "aks/msi-acrpull",
		RepositoryURL:       "",
		MasterBranch:        "",
	},
	"Backend": {
		Name:                "Backend",
		ImagePullRegistry:   "arohcpsvcdev.azurecr.io",
		ImagePullRepository: "arohcpbackend",
		RepositoryURL:       "https://github.com/Azure/ARO-HCP",
		MasterBranch:        "main",
		LatencyThreshold:    teamLatency,
	},
	"Backplane": {
		Name:                "Backplane",
		ImagePullRegistry:   "quay.io",
		ImagePullRepository: "app-sre/backplane-api",
		RepositoryURL:       "https://gitlab.cee.redhat.com/service/backplane-api",
		MasterBranch:        "master",
	},
	"Cluster Service": {
		Name:                "Cluster Service",
		ImagePullRegistry:   "quay.io",
		ImagePullRepository: "app-sre/uhc-clusters-service",
		RepositoryURL:       "https://gitlab.cee.redhat.com/service/uhc-clusters-service",
		MasterBranch:        "master",
		LatencyThreshold:    orgLatency,
	},
	"Frontend": {
		Name:                "Frontend",
		ImagePullRegistry:   "arohcpsvcdev.azurecr.io",
		ImagePullRepository: "arohcpfrontend",
		RepositoryURL:       "https://github.com/Azure/ARO-HCP",
		MasterBranch:        "main",
		LatencyThreshold:    teamLatency,
	},
	"Hypershift": {
		Name:                "Hypershift",
		ImagePullRegistry:   "quay.io",
		ImagePullRepository: "acm-d/rhtap-hypershift-operator",
		RepositoryURL:       "https://github.com/openshift/hypershift",
		MasterBranch:        "main",
		LatencyThreshold:    companyLatency,
	},
	"Maestro": {
		Name:                "Maestro",
		ImagePullRegistry:   "quay.io",
		ImagePullRepository: "redhat-user-workloads/maestro-rhtap-tenant/maestro/maestro",
		RepositoryURL:       "https://github.com/openshift-online/maestro/",
		MasterBranch:        "main",
	},
	"MCE": {
		Name:                "MCE",
		ImagePullRegistry:   "arohcpsvcdev.azurecr.io",
		ImagePullRepository: "multicluster-engine/mce-operator-bundle",
		RepositoryURL:       "https://github.com/stolostron/mce-operator-bundle",
		MasterBranch:        "main",
	},
	"OcMirror": {
		Name:                "OcMirror",
		ImagePullRegistry:   "arohcpsvcdev.azurecr.io",
		ImagePullRepository: "image-sync/oc-mirror",
		RepositoryURL:       "https://github.com/openshift/oc-mirror",
		MasterBranch:        "main",
	},
	"Package Operator Package": {
		Name:                "Package Operator Package",
		ImagePullRegistry:   "quay.io",
		ImagePullRepository: "package-operator/package-operator-package",
		RepositoryURL:       "https://github.com/package-operator/package-operator",
		MasterBranch:        "main",
	},
	"Package Operator Manager": {
		Name:                "Package Operator Manager",
		ImagePullRegistry:   "quay.io",
		ImagePullRepository: "package-operator/package-operator-manager",
		RepositoryURL:       "https://github.com/package-operator/package-operator",
		MasterBranch:        "main",
	},
	"Package Operator Remote Phase Manager": {
		Name:                "Package Operator Remote Phase Manager",
		ImagePullRegistry:   "quay.io",
		ImagePullRepository: "package-operator/remote-phase-manager",
		RepositoryURL:       "https://github.com/package-operator/package-operator",
		MasterBranch:        "main",
	},
	"Management Prometheus Spec": {
		Name:                "Management Prometheus Spec",
		ImagePullRegistry:   "mcr.microsoft.com/oss/v2",
		ImagePullRepository: "prometheus/prometheus",
		RepositoryURL:       "",
		MasterBranch:        "",
		LatencyThreshold:    worldLatency,
	},
	"Service Prometheus Spec": {
		Name:                "Service Prometheus Spec",
		ImagePullRegistry:   "mcr.microsoft.com/oss/v2",
		ImagePullRepository: "prometheus/prometheus",
		RepositoryURL:       "",
		MasterBranch:        "",
		LatencyThreshold:    worldLatency,
	},
}

// imagePullLocationForName returns the registry and repository for a given image name, or an error if the name isn't recognized.
func imagePullLocationForName(name string) (string, string, error) {
	info, exists := HardcodedComponents[name]
	if !exists {
		return "", "", fmt.Errorf("image pull location not found for image name %q", name)
	}
	return info.ImagePullRegistry, info.ImagePullRepository, nil
}

// credentialFile returns the filename in the credential directory to use for the image pull.
// empty means to use the system configured dockerconfig.
func credentialFile(imagePullSpec string) string {
	switch {
	case strings.HasPrefix(imagePullSpec, "quay.io/app-sre/"):
		return "quay-repository-app-sre-dockerconfig.json"
	case strings.HasPrefix(imagePullSpec, "quay.io/acm-d/"):
		return "quay-repository-acm-d-dockerconfig.json"
	case strings.HasPrefix(imagePullSpec, "arohcpsvcdev.azurecr.io/"):
		return "arohcpsvcdev-dockerconfig.json"
	default:
		return ""
	}
}
