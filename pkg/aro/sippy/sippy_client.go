package sippy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"k8s.io/klog/v2"
)

type SippyQueryStruct struct {
	Items        []SippyQueryItem `json:"items"`
	LinkOperator string           `json:"linkOperator"`
}

type SippyQueryItem struct {
	ColumnField   string `json:"columnField"`
	Not           bool   `json:"not"`
	OperatorValue string `json:"operatorValue"`
	Value         string `json:"value"`
}

type JobRunPaginationResult struct {
	Rows      []JobRun `json:"rows"`
	PageSize  int64    `json:"page_size"`
	Page      int      `json:"page"`
	TotalRows int64    `json:"total_rows"`
}

type JobRun struct {
	ID                    int              `json:"id"`
	BriefName             string           `json:"brief_name"`
	Variants              []string         `json:"variants" gorm:"type:text[]"`
	Tags                  []string         `json:"tags" gorm:"type:text[]"`
	TestGridURL           string           `json:"test_grid_url"`
	ProwID                uint             `json:"prow_id"`
	Job                   string           `json:"job"`
	Cluster               string           `json:"cluster"`
	URL                   string           `json:"url"`
	TestFlakes            int              `json:"test_flakes"`
	FlakedTestNames       []string         `json:"flaked_test_names" gorm:"type:text[]"`
	TestFailures          int              `json:"test_failures"`
	FailedTestNames       []string         `json:"failed_test_names" gorm:"type:text[]"`
	Failed                bool             `json:"failed"`
	InfrastructureFailure bool             `json:"infrastructure_failure"`
	KnownFailure          bool             `json:"known_failure"`
	Succeeded             bool             `json:"succeeded"`
	Timestamp             int64            `json:"timestamp"`
	OverallResult         JobOverallResult `json:"overall_result"`
	PullRequestOrg        string           `json:"pull_request_org"`
	PullRequestRepo       string           `json:"pull_request_repo"`
	PullRequestLink       string           `json:"pull_request_link"`
	PullRequestSHA        string           `json:"pull_request_sha"`
	PullRequestAuthor     string           `json:"pull_request_author"`
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

// sippyRelease is the name of the release in sippy, NOT ARO HCP.  Sippy's releases are our environments.
func ListJobRunsForEnvironment(ctx context.Context, sippyRelease string) ([]JobRun, error) {
	defaultTransport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			// we aren't sending anything sensitive.  Fix this before you do.
			InsecureSkipVerify: true,
		},
	}
	sippyClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: defaultTransport,
	}

	currURL := &url.URL{
		Scheme: "https",
		Host:   "sippy.dptools.openshift.org",
		Path:   "api/jobs/runs",
	}
	queryParams := currURL.Query()
	queryParams.Add("release", sippyRelease)
	queryParams.Add("period", "default")
	// have a look in openshift/api to see how we can further filter if we desire
	//filterJSON, err := json.Marshal(currQuery)
	//if err != nil {
	//	return nil, err
	//}
	//queryParams.Add("filter", string(filterJSON))
	currURL.RawQuery = queryParams.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, currURL.String(), nil)
	if err != nil {
		return nil, err
	}

	response, err := sippyClient.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("error getting sippy results (status=%d) for: %v", response.StatusCode, currURL.String())
	}
	queryResultBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	response.Body.Close()

	paginatedJobRuns := &JobRunPaginationResult{}
	if err := json.Unmarshal(queryResultBytes, paginatedJobRuns); err != nil {
		return nil, fmt.Errorf("error parsing sippy results (status=%d) for: %v", response.StatusCode, err)
	}
	if paginatedJobRuns.TotalRows != paginatedJobRuns.PageSize {
		klog.InfoS("more than one page of results")
	}

	return paginatedJobRuns.Rows, nil
}
