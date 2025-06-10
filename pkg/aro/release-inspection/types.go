package release_inspection

import (
	"sort"

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
		environmentNames.Insert(set.KeySet(currReleaseInfo.environmentToEnvironmentInfo).UnsortedList()...)
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
	ReleaseName                  string
	environmentToEnvironmentInfo map[string]*ReleaseEnvironmentInfo
}

func (r *ReleaseInfo) GetEnvironmentFilenames() []string {
	if r == nil {
		return nil
	}
	environmentNames := set.KeySet(r.environmentToEnvironmentInfo)
	return environmentNames.SortedList()
}

func (r *ReleaseInfo) addEnvironment(environmentInfo *ReleaseEnvironmentInfo) {
	if r.environmentToEnvironmentInfo == nil {
		r.environmentToEnvironmentInfo = make(map[string]*ReleaseEnvironmentInfo)
	}
	r.environmentToEnvironmentInfo[environmentInfo.EnvironmentFilename] = environmentInfo
}

func (r *ReleaseInfo) GetInfoForEnvironment(environment string) *ReleaseEnvironmentInfo {
	if r == nil {
		return nil
	}
	return r.environmentToEnvironmentInfo[environment]
}
