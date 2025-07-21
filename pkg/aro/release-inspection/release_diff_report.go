package release_inspection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	arohcpapi "github.com/openshift-online/service-status/pkg/apis/aro-hcp"
	"k8s.io/klog/v2"
)

type ReleaseDiffReport struct {
	// these fields are for input, not output

	imageInfoAccessor ImageInfoAccessor
	releaseName       string
	environments      []string
	repoDir           string
	prevReleaseInfo   *ReleaseInfo
}

func NewReleaseDiffReport(imageInfoAccessor ImageInfoAccessor, releaseName string, repoDir string, environments []string, prevReleaseInfo *ReleaseInfo) *ReleaseDiffReport {
	return &ReleaseDiffReport{
		imageInfoAccessor: imageInfoAccessor,
		releaseName:       releaseName,
		repoDir:           repoDir,
		environments:      environments,
		prevReleaseInfo:   prevReleaseInfo,
	}
}

func (r *ReleaseDiffReport) ReleaseInfoForAllEnvironments(ctx context.Context) (*ReleaseInfo, error) {
	ret := &ReleaseInfo{
		ReleaseName: r.releaseName,
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

	for _, environmentFilename := range r.environments {
		localLogger := klog.FromContext(ctx)
		localLogger = klog.LoggerWithValues(localLogger, "configFile", environmentFilename)
		localCtx := klog.NewContext(ctx, localLogger)

		configJSON := []byte{}
		var config *arohcpapi.ConfigSchemaJSON // may be an overlay
		switch {
		case environmentFilename == "int" || environmentFilename == "stg" || environmentFilename == "prod":
			intOverlayMap := allConfigOverlays.Clouds["public"].(map[string]interface{})["environments"].(map[string]interface{})["int"].(map[string]interface{})["defaults"]
			configJSON, err = json.MarshalIndent(intOverlayMap, "", "    ")
			if err != nil {
				return nil, fmt.Errorf("failed to marshal JSON: %w", err)
			}
			//fmt.Println(string(intOverlayJSON))
			if err := json.Unmarshal(configJSON, &config); err != nil {
				return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
			}

		default:
			panic("TODO we may later add parsing of rendered files")
		}
		//fullPath := filepath.Join(r.repoDir, "config", environmentFilename)
		//jsonBytes, err := os.ReadFile(fullPath)
		//if errors.Is(err, os.ErrNotExist) {
		//	continue
		//}
		//if err != nil {
		//	return nil, fmt.Errorf("failed to read file %s: %w", fullPath, err)
		//}

		prevReleaseEnvironmentInfo := r.prevReleaseInfo.GetInfoForEnvironment(environmentFilename)
		currReleaseEnvironmentInfo, err := r.releaseMarkdownForConfigJSON(localCtx, environmentFilename, configJSON, prevReleaseEnvironmentInfo)
		if err != nil {
			// the schema in ARO-HCP is changing incompatibly, so we are not guaranteed to be able to parse older releases
			localLogger.Error(err, "failed to release markdown for config JSON.  Continuing...")
			continue
			//return nil, fmt.Errorf("failed to create markdown for %s: %w", fullPath, err)
		}
		ret.addEnvironment(currReleaseEnvironmentInfo)
	}

	return ret, nil
}

func (r *ReleaseDiffReport) releaseMarkdownForConfigJSON(ctx context.Context, environmentName string, currReleaseEnvironmentJSON []byte, prevReleaseEnvironmentInfo *ReleaseEnvironmentInfo) (*ReleaseEnvironmentInfo, error) {
	config := &arohcpapi.ConfigSchemaJSON{}
	err := json.Unmarshal(currReleaseEnvironmentJSON, config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	ret, err := r.releaseMarkdownForConfig(ctx, environmentName, config, prevReleaseEnvironmentInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to create markdown for %s: %w", r.releaseName, err)
	}
	return ret, nil
}

func (r *ReleaseDiffReport) releaseMarkdownForConfig(ctx context.Context, environmentName string, config *arohcpapi.ConfigSchemaJSON, prevReleaseEnvironmentInfo *ReleaseEnvironmentInfo) (*ReleaseEnvironmentInfo, error) {
	logger := klog.FromContext(ctx)
	logger.Info("Scraping info")

	currConfigInfo, err := scrapeInfoForAROHCPConfig(ctx, r.imageInfoAccessor, r.releaseName, environmentName, config, prevReleaseEnvironmentInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to create markdown for %s: %w", r.releaseName, err)
	}

	return currConfigInfo, nil
}

func must[T any](ret T, err error) T {
	if err != nil {
		panic(err)
	}
	return ret
}
