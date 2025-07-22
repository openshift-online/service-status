package release_inspection

import "fmt"

// imagePullLocationForName returns the registry and repository for a given image name, or an error if the name isn't recognized.
func imagePullLocationForName(name string) (string, string, error) {
	switch name {
	case "Cluster Service":
		return "quay.io", "app-sre/uhc-clusters-service", nil
	case "Hypershift":
		return "quay.io", "acm-d/rhtap-hypershift-operator", nil
	case "Backend":
		return "arohcpsvcdev.azurecr.io", "arohcpbackend", nil
	case "Backplane":
		return "quay.io", "app-sre/backplane-api", nil
	case "Frontend":
		return "arohcpsvcdev.azurecr.io", "arohcpfrontend", nil
	case "OcMirror":
		return "arohcpsvcdev.azurecr.io", "image-sync/oc-mirror", nil
	case "Maestro":
		return "quay.io", "redhat-user-workloads/maestro-rhtap-tenant/maestro/maestro", nil
	case "Management Prometheus Spec":
		return "mcr.microsoft.com/oss/v2", "prometheus/prometheus", nil
	case "ACR Pull":
		return "mcr.microsoft.com", "aks/msi-acrpull", nil
	case "Service Prometheus Spec":
		return "mcr.microsoft.com/oss/v2", "prometheus/prometheus", nil
	default:
		return "", "", fmt.Errorf("image pull location not found for image name %q", name)
	}
}
