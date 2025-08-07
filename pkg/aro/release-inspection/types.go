package release_inspection

import (
	"sort"

	"github.com/openshift-online/service-status/pkg/apis/status"
	"k8s.io/utils/set"
)

type ReleasesInfo struct {
	releaseToInfo map[string]*ReleaseInfo
}

func (r *ReleasesInfo) GetReleaseNames() []string {
	if r == nil {
		return nil
	}

	releasesOldestFirst := set.KeySet(r.releaseToInfo).SortedList()
	sort.Sort(sort.Reverse(sort.StringSlice(releasesOldestFirst)))
	return releasesOldestFirst
}

func (r *ReleasesInfo) GetEnvironmentFilenames() []string {
	if r == nil {
		return nil
	}
	environmentNames := set.Set[string]{}
	for _, currReleaseInfo := range r.releaseToInfo {
		environmentNames.Insert(set.KeySet(currReleaseInfo.environmentToEnvironmentRelease).UnsortedList()...)
	}
	return environmentNames.SortedList()
}

func (r *ReleasesInfo) AddReleaseInfo(newReleaseInfo *ReleaseInfo) {
	if r.releaseToInfo == nil {
		r.releaseToInfo = make(map[string]*ReleaseInfo)
	}
	r.releaseToInfo[newReleaseInfo.ReleaseName] = newReleaseInfo
}

func (r *ReleasesInfo) GetReleaseInfo(release string) *ReleaseInfo {
	if r == nil {
		return nil
	}
	return r.releaseToInfo[release]
}

type ReleaseInfo struct {
	ReleaseName                     string
	environmentToEnvironmentRelease map[string]*status.EnvironmentRelease
}

func (r *ReleaseInfo) GetEnvironmentNames() []string {
	if r == nil {
		return nil
	}
	environmentNames := set.KeySet(r.environmentToEnvironmentRelease)
	return environmentNames.SortedList()
}

func (r *ReleaseInfo) addEnvironmentRelease(environmentInfo *status.EnvironmentRelease) {
	if r.environmentToEnvironmentRelease == nil {
		r.environmentToEnvironmentRelease = make(map[string]*status.EnvironmentRelease)
	}
	r.environmentToEnvironmentRelease[environmentInfo.Environment] = environmentInfo
}

func (r *ReleaseInfo) GetEnvironmentRelease(environment string) *status.EnvironmentRelease {
	if r == nil {
		return nil
	}
	return r.environmentToEnvironmentRelease[environment]
}
