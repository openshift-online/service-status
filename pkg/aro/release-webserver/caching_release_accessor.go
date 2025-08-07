package release_webserver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/openshift-online/service-status/pkg/apis/status"
	"k8s.io/utils/clock"
)

type cachingReleaseAccessor struct {
	delegate ReleaseAccessor
	clock    clock.Clock

	listReleases                     *stringBasedResultTimeBasedCacher[*status.ReleaseList]
	listEnvironments                 *stringBasedResultTimeBasedCacher[[]string]
	getReleaseInfoForAllEnvironments *stringBasedResultTimeBasedCacher[*status.ReleaseDetails]
	getReleaseEnvironmentInfo        *stringBasedResultTimeBasedCacher[*status.EnvironmentRelease]
	getReleaseEnvironmentDiff        *stringBasedResultTimeBasedCacher[*status.EnvironmentReleaseDiff]
}

func NewCachingReleaseAccessor(delegate ReleaseAccessor, clock clock.Clock) ReleaseAccessor {
	return &cachingReleaseAccessor{
		delegate: delegate,
		clock:    clock,
		listReleases: &stringBasedResultTimeBasedCacher[*status.ReleaseList]{
			delegate: noKeyAdapter(delegate.ListReleases),
			clock:    clock,
		},
		listEnvironments: &stringBasedResultTimeBasedCacher[[]string]{
			delegate: noKeyAdapter(delegate.ListEnvironments),
			clock:    clock,
		},
		getReleaseInfoForAllEnvironments: &stringBasedResultTimeBasedCacher[*status.ReleaseDetails]{
			delegate: delegate.GetReleaseInfoForAllEnvironments,
			clock:    clock,
		},
		getReleaseEnvironmentInfo: &stringBasedResultTimeBasedCacher[*status.EnvironmentRelease]{
			delegate: delegate.GetReleaseEnvironmentInfo,
			clock:    clock,
		},
		getReleaseEnvironmentDiff: &stringBasedResultTimeBasedCacher[*status.EnvironmentReleaseDiff]{
			delegate: twoStringAdapter(delegate.GetReleaseEnvironmentDiff),
			clock:    clock,
		},
	}
}

func noKeyAdapter[T any](fn func(ctx context.Context) (T, error)) func(ctx context.Context, key string) (T, error) {
	return func(ctx context.Context, key string) (T, error) {
		return fn(ctx)
	}
}

func twoStringAdapter[T any](fn func(ctx context.Context, part1, part2 string) (T, error)) func(ctx context.Context, key string) (T, error) {
	return func(ctx context.Context, key string) (T, error) {
		part1, part2, found := strings.Cut(key, "###")
		if !found {
			panic("caller error: key should contain ### separator")
		}
		return fn(ctx, part1, part2)
	}
}

type stringBasedResultTimeBasedCacher[T any] struct {
	delegate func(ctx context.Context, key string) (T, error)
	clock    clock.Clock

	lock        sync.RWMutex
	lastRefresh map[string]time.Time
	results     map[string]T
}

func (r *stringBasedResultTimeBasedCacher[T]) Do(ctx context.Context, key string) (T, error) {
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

	curr, err := r.delegate(ctx, key)
	if err != nil {
		return curr, err
	}

	if r.results == nil {
		r.results = make(map[string]T)
	}
	if r.lastRefresh == nil {
		r.lastRefresh = make(map[string]time.Time)
	}
	r.lastRefresh[key] = r.clock.Now()
	r.results[key] = curr
	return r.results[key], nil
}

func (r *cachingReleaseAccessor) ListEnvironments(ctx context.Context) ([]string, error) {
	return r.listEnvironments.Do(ctx, "")
}

func (r *cachingReleaseAccessor) ListReleases(ctx context.Context) (*status.ReleaseList, error) {
	return r.listReleases.Do(ctx, "")
}

func (r *cachingReleaseAccessor) GetReleaseEnvironmentInfo(ctx context.Context, environmentReleaseName string) (*status.EnvironmentRelease, error) {
	return r.getReleaseEnvironmentInfo.Do(ctx, environmentReleaseName)
}

func (r *cachingReleaseAccessor) GetReleaseInfoForAllEnvironments(ctx context.Context, releaseName string) (*status.ReleaseDetails, error) {
	return r.getReleaseInfoForAllEnvironments.Do(ctx, releaseName)
}

func (r *cachingReleaseAccessor) GetReleaseEnvironmentDiff(ctx context.Context, environmentReleaseName, otherEnvironmentReleaseName string) (*status.EnvironmentReleaseDiff, error) {
	return r.getReleaseEnvironmentDiff.Do(ctx, fmt.Sprintf("%v###%v", environmentReleaseName, otherEnvironmentReleaseName))
}
