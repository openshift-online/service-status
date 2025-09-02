package status

import (
	"time"
)

type TypeMeta struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
}

type Environment struct {
	TypeMeta `json:",inline"`
	Name     string `json:"name"`
}

type EnvironmentList struct {
	TypeMeta `json:",inline"`
	Items    []Environment `json:"items"`
}

type Release struct {
	TypeMeta `json:",inline"`
	Name     string `json:"name"`
	SHA      string `json:"sha"`
}

type ReleaseList struct {
	TypeMeta `json:",inline"`
	Items    []Release `json:"items"`
}

type ReleaseDetails struct {
	TypeMeta `json:",inline"`
	Name     string `json:"name"`
	SHA      string `json:"sha"`

	Environments map[string]*EnvironmentRelease `json:"environments"`
}

type JobRunResults struct {
	OverallResult JobOverallResult `json:"overall_result"`
	URL           string           `json:"url"`
}

type JobOverallResult string

const (
	JobSucceeded             JobOverallResult = "S"
	JobRunning               JobOverallResult = "R"
	JobInfrastructureFailure JobOverallResult = "N"
	JobInstallFailure        JobOverallResult = "I"
	JobUpgradeFailure        JobOverallResult = "U"
	JobTestFailure           JobOverallResult = "F"
	JobFailureBeforeSetup    JobOverallResult = "n"
	JobAborted               JobOverallResult = "A"
	JobUnknown               JobOverallResult = "f"
)

type EnvironmentRelease struct {
	TypeMeta              `json:",inline"`
	Name                  string                `json:"name"`
	ReleaseName           string                `json:"releaseName"`
	SHA                   string                `json:"sha"`
	Environment           string                `json:"environment"`
	Components            map[string]*Component `json:"components"`
	ProbableJobRunResults []JobRunResults       `json:"probableJobRunResults"`
}

type EnvironmentReleaseList struct {
	TypeMeta `json:",inline"`
	Items    []EnvironmentRelease `json:"items"`
}

type Component struct {
	Name                     string `json:"name"`
	ImageInfo                ContainerImage
	ImageCreationTime        *time.Time `json:"imageCreationTime,omitempty"`
	RepoURL                  *string    `json:"RepoURL"`
	SourceSHA                string     `json:"sourceSHA"`
	PermanentURLForSourceSHA *string    `json:"permanentURLForSourceSHA,omitempty"`
}

type ContainerImage struct {
	Digest     string `json:"digest"`
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
}

type EnvironmentReleaseDiff struct {
	TypeMeta                    `json:",inline"`
	Name                        string `json:"name"`
	OtherEnvironmentReleaseName string `json:"otherEnvironmentReleaseName"`

	DifferentComponents map[string]*ComponentDiff `json:"differentComponents"`
}

type ComponentDiff struct {
	Name string `json:"name"`

	NumberOfChanges int               `json:"numberOfChanges"`
	Changes         []ComponentChange `json:"changes"`
}

type ComponentChange struct {
	ChangeType  string   `json:"changeType"`
	PRMerge     *PRMerge `json:"prMerge,omitempty"`
	Unavailable *string  `json:"unavailable,omitempty"`
}

type PRMerge struct {
	PRNumber      int32    `json:"PRNumber"`
	SHA           string   `json:"SHA"`
	ChangeSummary string   `json:"topLineCommitMessage"`
	JIRARefs      []string `json:"jiraRefs,omitempty"`
}
