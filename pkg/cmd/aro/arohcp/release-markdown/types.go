package release_markdown

import (
	"sort"

	"github.com/openshift-online/service-status/pkg/apis/status"
	"k8s.io/utils/set"
)

type AllReleasesDetails struct {
	releaseNameToDetails map[string]*status.ReleaseDetails
}

func (r *AllReleasesDetails) GetReleaseNames() []string {
	if r == nil {
		return nil
	}

	releasesOldestFirst := set.KeySet(r.releaseNameToDetails).SortedList()
	sort.Sort(sort.Reverse(sort.StringSlice(releasesOldestFirst)))
	return releasesOldestFirst
}

func (r *AllReleasesDetails) GetEnvironmentFilenames() []string {
	if r == nil {
		return nil
	}
	environmentNames := set.Set[string]{}
	for _, currReleaseInfo := range r.releaseNameToDetails {
		environmentNames.Insert(set.KeySet(currReleaseInfo.Environments).UnsortedList()...)
	}
	return environmentNames.SortedList()
}

func (r *AllReleasesDetails) AddReleaseDetails(newReleaseInfo *status.ReleaseDetails) {
	if r.releaseNameToDetails == nil {
		r.releaseNameToDetails = make(map[string]*status.ReleaseDetails)
	}
	r.releaseNameToDetails[newReleaseInfo.Name] = newReleaseInfo
}

func (r *AllReleasesDetails) GetReleaseInfo(release string) *status.ReleaseDetails {
	if r == nil {
		return nil
	}
	return r.releaseNameToDetails[release]
}
