package release_webserver

import (
	"context"
	"sync"
	"time"

	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"k8s.io/utils/clock"
)

type cachingReleaseAccessor struct {
	delegate ReleaseAccessor
	clock    clock.Clock

	listReleases     *singleResultTimeBasedCacher[[]Release]
	listEnvironments *singleResultTimeBasedCacher[[]string]
}

type singleResultTimeBasedCacher[T any] struct {
	delegate func(ctx context.Context) (T, error)
	clock    clock.Clock

	lock        sync.RWMutex
	lastRefresh time.Time
	result      T
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
	}
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

func (r *cachingReleaseAccessor) ListEnvironments(ctx context.Context) ([]string, error) {
	return r.listEnvironments.Do(ctx)
}

func (r *cachingReleaseAccessor) ListReleases(ctx context.Context) ([]Release, error) {
	return r.listReleases.Do(ctx)
}

func (r *cachingReleaseAccessor) GetReleaseEnvironmentInfo(ctx context.Context, release Release, environment string) (*release_inspection.ReleaseEnvironmentInfo, error) {
	return r.delegate.GetReleaseEnvironmentInfo(ctx, release, environment)
}

func (r *cachingReleaseAccessor) GetReleaseInfoForAllEnvironments(ctx context.Context, release Release) (*release_inspection.ReleaseInfo, error) {
	return r.delegate.GetReleaseInfoForAllEnvironments(ctx, release)
}
