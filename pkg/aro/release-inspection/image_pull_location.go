package release_inspection

import (
	"fmt"
	"strings"
)

type HardcodedComponentInfo struct {
	Name                string
	ImagePullRegistry   string
	ImagePullRepository string
	RepositoryURL       string
	MasterBranch        string
}

var HardcodedComponents = map[string]HardcodedComponentInfo{
	"ACM Operator": {
		Name:                "ACM Operator",
		ImagePullRegistry:   "registry.redhat.io",
		ImagePullRepository: "rhacm2/acm-operator-bundle",
		RepositoryURL:       "",
		MasterBranch:        "",
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
	},
	"Frontend": {
		Name:                "Frontend",
		ImagePullRegistry:   "arohcpsvcdev.azurecr.io",
		ImagePullRepository: "arohcpfrontend",
		RepositoryURL:       "https://github.com/Azure/ARO-HCP",
		MasterBranch:        "main",
	},
	"Hypershift": {
		Name:                "Hypershift",
		ImagePullRegistry:   "quay.io",
		ImagePullRepository: "acm-d/rhtap-hypershift-operator",
		RepositoryURL:       "https://github.com/openshift/hypershift",
		MasterBranch:        "main",
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
		ImagePullRegistry:   "registry.redhat.io",
		ImagePullRepository: "multicluster-engine/mce-operator-bundle",
		RepositoryURL:       "",
		MasterBranch:        "",
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
	},
	"Service Prometheus Spec": {
		Name:                "Service Prometheus Spec",
		ImagePullRegistry:   "mcr.microsoft.com/oss/v2",
		ImagePullRepository: "prometheus/prometheus",
		RepositoryURL:       "",
		MasterBranch:        "",
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
