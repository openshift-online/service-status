package release_inspection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift-online/service-status/pkg/apis/status"
	"gopkg.in/yaml.v3"

	arohcpapi "github.com/openshift-online/service-status/pkg/apis/aro-hcp"
	"k8s.io/klog/v2"
)

type ReleaseDiffReport struct {
	// these fields are for input, not output

	imageInfoAccessor ImageInfoAccessor
	releaseName       string
	releaseSHA        string
	environments      []string
	repoDir           string
}

func NewReleaseDiffReport(imageInfoAccessor ImageInfoAccessor, releaseName, releaseSHA string, repoDir string, environments []string) *ReleaseDiffReport {
	return &ReleaseDiffReport{
		imageInfoAccessor: imageInfoAccessor,
		releaseName:       releaseName,
		releaseSHA:        releaseSHA,
		repoDir:           repoDir,
		environments:      environments,
	}
}

func (r *ReleaseDiffReport) ReleaseInfoForAllEnvironments(ctx context.Context) (*status.ReleaseDetails, error) {
	ret := &status.ReleaseDetails{
		TypeMeta: status.TypeMeta{
			Kind:       "ReleaseDetails",
			APIVersion: "service-status.hcm.openshift.io/v1",
		},
		Name:         r.releaseName,
		SHA:          r.releaseSHA,
		Environments: map[string]*status.EnvironmentRelease{},
	}

	baseConfigFilename := filepath.Join(r.repoDir, "config", "config.yaml")
	baseConfigBytes, err := os.ReadFile(baseConfigFilename)
	if errors.Is(err, os.ErrNotExist) {
		return ret, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", baseConfigFilename, err)
	}
	baseConfig := &arohcpapi.ConfigSchemaJSON{}
	if err := yaml.Unmarshal(baseConfigBytes, baseConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config.yaml: %w", err)
	}

	configOverlayFilename := filepath.Join(r.repoDir, "config", "config.msft.clouds-overlay.yaml")
	configOverlayJSONBytes, err := os.ReadFile(configOverlayFilename)
	if errors.Is(err, os.ErrNotExist) {
		return ret, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", configOverlayFilename, err)
	}
	allConfigOverlays := &arohcpapi.ConfigMetaSchemaJSON{}
	if err := yaml.Unmarshal(configOverlayJSONBytes, allConfigOverlays); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	for _, environmentName := range r.environments {
		localLogger := klog.FromContext(ctx)
		localLogger = klog.LoggerWithValues(localLogger, "configFile", environmentName)

		var overlayConfig *arohcpapi.ConfigSchemaJSON // may be an overlay
		switch {
		case environmentName == "int" || environmentName == "stg" || environmentName == "prod":
			intOverlayMap := allConfigOverlays.Clouds["public"].(map[string]interface{})["environments"].(map[string]interface{})[environmentName].(map[string]interface{})["defaults"]
			overlayConfigJSON, err := json.MarshalIndent(intOverlayMap, "", "    ")
			if err != nil {
				return nil, fmt.Errorf("failed to marshal JSON: %w", err)
			}
			if err := json.Unmarshal(overlayConfigJSON, &overlayConfig); err != nil {
				return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
			}

		default:
			panic(fmt.Sprintf("TODO we may later add parsing of rendered files: %v", environmentName))
		}

		currReleaseEnvironmentInfo, err := scrapeInfoForAROHCPConfig(ctx, r.imageInfoAccessor, r.releaseName, r.releaseSHA, environmentName, overlayConfig)
		if err != nil {
			// the schema in ARO-HCP is changing incompatibly, so we are not guaranteed to be able to parse older releases
			localLogger.Error(err, "failed to read ARO HCP config for environment=%q release=%q.  Continuing...", environmentName, r.releaseName)
			continue
		}
		ret.Environments[currReleaseEnvironmentInfo.Environment] = currReleaseEnvironmentInfo
	}

	return ret, nil
}
