package release_webserver

import (
	"context"
	"sync"
	"time"

	"github.com/openshift-online/service-status/pkg/apis/status"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"k8s.io/utils/clock"
)

type cachingReleaseAccessor struct {
	delegate ReleaseAccessor
	clock    clock.Clock

	listReleases              *singleResultTimeBasedCacher[[]Release]
	listEnvironments          *singleResultTimeBasedCacher[[]string]
	getReleaseEnvironmentInfo *releaseEnvironmentBasedResultTimeBasedCacher[*release_inspection.ReleaseEnvironmentInfo]
}

func NewCachingReleaseAccessor(delegate ReleaseAccessor, clock clock.Clock) ReleaseAccessor {
	return &cachingReleaseAccessor{
		delegate: delegate,
		clock:    clock,
		listReleases: &singleResultTimeBasedCacher[[]Release]{
			delegate: delegate.ListReleases,
			clock:    clock,
		},
		listEnvironments: &singleResultTimeBasedCacher[[]string]{
			delegate: delegate.ListEnvironments,
			clock:    clock,
		},
		getReleaseEnvironmentInfo: &releaseEnvironmentBasedResultTimeBasedCacher[*release_inspection.ReleaseEnvironmentInfo]{
			delegate: delegate.GetReleaseEnvironmentInfo,
			clock:    clock,
		},
	}
}

type singleResultTimeBasedCacher[T any] struct {
	delegate func(ctx context.Context) (T, error)
	clock    clock.Clock

	lock        sync.RWMutex
	lastRefresh time.Time
	result      T
}

func (r *singleResultTimeBasedCacher[T]) Do(ctx context.Context) (T, error) {
	r.lock.RLock()
	if r.clock.Since(r.lastRefresh) < 1*time.Hour {
		defer r.lock.RUnlock()
		return r.result, nil
	}
	r.lock.RUnlock()

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.clock.Since(r.lastRefresh) < 1*time.Hour {
		return r.result, nil
	}

	curr, err := r.delegate(ctx)
	if err != nil {
		return curr, err
	}

	r.lastRefresh = r.clock.Now()
	r.result = curr
	return r.result, nil
}

type releaseEnvironmentBasedResultTimeBasedCacher[T any] struct {
	delegate func(ctx context.Context, release Release, environment string) (T, error)
	clock    clock.Clock

	lock        sync.RWMutex
	lastRefresh map[releaseEnvironmentKey]time.Time
	results     map[releaseEnvironmentKey]T
}
type releaseEnvironmentKey struct {
	Release     Release
	Environment string
}

func (r *releaseEnvironmentBasedResultTimeBasedCacher[T]) Do(ctx context.Context, release Release, environment string) (T, error) {
	key := releaseEnvironmentKey{
		Release:     release,
		Environment: environment,
	}
	r.lock.RLock()
	if r.clock.Since(r.lastRefresh[key]) < 1*time.Hour {
		defer r.lock.RUnlock()
		return r.results[key], nil
	}
	r.lock.RUnlock()

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.clock.Since(r.lastRefresh[key]) < 1*time.Hour {
		return r.results[key], nil
	}

	curr, err := r.delegate(ctx, release, environment)
	if err != nil {
		return curr, err
	}

	if r.results == nil {
		r.results = make(map[releaseEnvironmentKey]T)
	}
	if r.lastRefresh == nil {
		r.lastRefresh = make(map[releaseEnvironmentKey]time.Time)
	}
	r.lastRefresh[key] = r.clock.Now()
	r.results[key] = curr
	return r.results[key], nil
}

func (r *cachingReleaseAccessor) ListEnvironments(ctx context.Context) ([]string, error) {
	return r.listEnvironments.Do(ctx)
}

func (r *cachingReleaseAccessor) ListReleases(ctx context.Context) ([]Release, error) {
	return r.listReleases.Do(ctx)
}

func (r *cachingReleaseAccessor) GetReleaseEnvironmentInfo(ctx context.Context, release Release, environment string) (*release_inspection.ReleaseEnvironmentInfo, error) {
	return r.getReleaseEnvironmentInfo.Do(ctx, release, environment)
}

func (r *cachingReleaseAccessor) GetReleaseInfoForAllEnvironments(ctx context.Context, release Release) (*release_inspection.ReleaseInfo, error) {
	return r.delegate.GetReleaseInfoForAllEnvironments(ctx, release)
}

func (r *cachingReleaseAccessor) GetReleaseEnvironmentDiff(ctx context.Context, release Release, environment string, otherRelease Release, otherEnvironment string) (*status.EnvironmentReleaseDiff, error) {
	return r.delegate.GetReleaseEnvironmentDiff(ctx, release, environment, otherRelease, otherEnvironment)
}
