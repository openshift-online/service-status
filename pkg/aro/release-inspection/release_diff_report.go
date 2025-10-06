package release_inspection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"dario.cat/mergo"
	"github.com/openshift-online/service-status/pkg/apis/status"
	"sigs.k8s.io/yaml"

	arohcpapi "github.com/openshift-online/service-status/pkg/apis/aro-hcp"
	"k8s.io/klog/v2"
)

type EnvironmentReleaseLookupInformation struct {
	EnvironmentName    string
	ReleaseName        string
	ReleaseSHA         string
	InterestingContent map[string][]byte
}

// CompleteEnvironmentReleaseInput assumes the repo directory is already in the correct state for the release.  It inspects
// the content and returns a set of "files" that are the merged result of the repo state.
// Currently this is the config.yaml with the correct config.msft.clouds-overlay.yaml region overlayed.
// This could later be extended to include other files.
func CompleteEnvironmentReleaseInput(ctx context.Context, repoDir, environmentName string) (map[string][]byte, error) {
	baseConfigFilename := filepath.Join(repoDir, "config", "config.yaml")
	baseConfigBytes, err := os.ReadFile(baseConfigFilename)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", baseConfigFilename, err)
	}
	// it's not actually yaml (eek).  Coerce
	baseConfigBytes = bytes.ReplaceAll(baseConfigBytes, []byte("{{ .ev2.availabilityZoneCount }}"), []byte("2"))
	// this existed in a1afdeea19d3d4190d1cae30ce639be338d9f7a8, maybe someone will actually fix the code.
	baseConfigBytes = bytes.ReplaceAll(baseConfigBytes, []byte("environmentName: {{ .ctx.environment }}"), []byte(`environmentName: ANY_KEY`))
	baseConfigMap := map[string]interface{}{}
	if err := yaml.Unmarshal(baseConfigBytes, &baseConfigMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config.yaml: %w", err)
	}
	baseConfigMap = baseConfigMap["defaults"].(map[string]interface{})

	configOverlayFilename := filepath.Join(repoDir, "config", "config.msft.clouds-overlay.yaml")
	configOverlayJSONBytes, err := os.ReadFile(configOverlayFilename)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", configOverlayFilename, err)
	}
	allConfigOverlays := &arohcpapi.ConfigMetaSchemaJSON{}
	if err := yaml.Unmarshal(configOverlayJSONBytes, allConfigOverlays); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	localLogger := klog.FromContext(ctx)
	localLogger = klog.LoggerWithValues(localLogger, "configFile", environmentName)

	overlayConfigMap := map[string]interface{}{}
	switch {
	case environmentName == "int" || environmentName == "stg" || environmentName == "prod":
		intOverlayMap := allConfigOverlays.Clouds["public"].(map[string]interface{})["environments"].(map[string]interface{})[environmentName].(map[string]interface{})["defaults"]
		overlayConfigJSON, err := json.MarshalIndent(intOverlayMap, "", "    ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON: %w", err)
		}
		if err := json.Unmarshal(overlayConfigJSON, &overlayConfigMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
		}

	default:
		panic(fmt.Sprintf("TODO we may later add parsing of rendered files: %q", environmentName))
	}

	if err := mergo.Merge(&overlayConfigMap, baseConfigMap); err != nil {
		return nil, fmt.Errorf("failed to merge base config with overlay: %w", err)
	}
	overlayConfigJSON, err := json.MarshalIndent(overlayConfigMap, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// virtual file
	ret := map[string][]byte{
		"virtual-config/environment-release-config.json": overlayConfigJSON,
	}

	return ret, nil
}

func ReleaseInfo(ctx context.Context, imageInfoAccessor ImageInfoAccessor, releaseLookupInformation *EnvironmentReleaseLookupInformation) (*status.EnvironmentRelease, error) {
	localLogger := klog.FromContext(ctx)
	localLogger = klog.LoggerWithValues(localLogger, "releaseLookupInformation", releaseLookupInformation)

	var overlayConfig *arohcpapi.ConfigSchemaJSON // may be an overlay
	if err := json.Unmarshal(releaseLookupInformation.InterestingContent["virtual-config/environment-release-config.json"], &overlayConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return scrapeInfoForAROHCPConfig(ctx, imageInfoAccessor, releaseLookupInformation.EnvironmentName, releaseLookupInformation.ReleaseName, releaseLookupInformation.ReleaseSHA, overlayConfig)
}
