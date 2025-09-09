package release_webserver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/openshift-online/service-status/pkg/apis/status"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
)

type cachingReleaseAccessor struct {
	selfLookupInstance ReleaseAccessor
	delegate           ReleaseAccessor
	clock              clock.Clock

	listEnvironments                      *stringBasedResultTimeBasedCacher[[]string]
	listEnvironmentReleases               *stringBasedResultTimeBasedCacher[*status.EnvironmentReleaseList]
	listEnvironmentReleasesForEnvironment *stringBasedResultTimeBasedCacher[*status.EnvironmentReleaseList]
	getEnvironmentRelease                 *stringBasedResultTimeBasedCacher[*status.EnvironmentRelease]
	getReleaseEnvironmentDiff             *stringBasedResultTimeBasedCacher[*status.EnvironmentReleaseDiff]
}

func NewCachingReleaseAccessor(delegate ReleaseAccessor, clock clock.Clock) ReleaseAccessor {
	ret := &cachingReleaseAccessor{
		delegate: delegate,
		clock:    clock,
		listEnvironments: &stringBasedResultTimeBasedCacher[[]string]{
			delegate: noKeyAdapter(delegate.ListEnvironments),
			clock:    clock,
		},
		listEnvironmentReleases: &stringBasedResultTimeBasedCacher[*status.EnvironmentReleaseList]{
			delegate: noKeyAdapter(delegate.ListEnvironmentReleases),
			clock:    clock,
		},
		listEnvironmentReleasesForEnvironment: &stringBasedResultTimeBasedCacher[*status.EnvironmentReleaseList]{
			delegate: delegate.ListEnvironmentReleasesForEnvironment,
			clock:    clock,
		},
		getEnvironmentRelease: &stringBasedResultTimeBasedCacher[*status.EnvironmentRelease]{
			delegate: delegate.GetEnvironmentRelease,
			clock:    clock,
		},
		getReleaseEnvironmentDiff: &stringBasedResultTimeBasedCacher[*status.EnvironmentReleaseDiff]{
			delegate: twoStringAdapter(delegate.GetReleaseEnvironmentDiff),
			clock:    clock,
		},
	}
	ret.SetSelfLookupInstance(ret)
	delegate.SetSelfLookupInstance(ret)
	return ret
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
	logger := klog.FromContext(ctx)

	r.lock.RLock()
	if r.clock.Since(r.lastRefresh[key]) < 1*time.Hour {
		defer r.lock.RUnlock()
		return r.results[key], nil
	}
	r.lock.RUnlock()

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.clock.Since(r.lastRefresh[key]) < 1*time.Hour {
		logger.Info("returning cached result", "key", key)
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

func (r *cachingReleaseAccessor) GetEnvironmentRelease(ctx context.Context, environmentReleaseName string) (*status.EnvironmentRelease, error) {
	allEnvironmentReleases, err := r.ListEnvironmentReleases(ctx)
	if err != nil {
		return nil, err
	}

	for _, curr := range allEnvironmentReleases.Items {
		if curr.Name == environmentReleaseName {
			return &curr, nil
		}
	}

	return nil, fmt.Errorf("environment release not found: %s", environmentReleaseName)
}

func (r *cachingReleaseAccessor) GetReleaseEnvironmentDiff(ctx context.Context, environmentReleaseName, otherEnvironmentReleaseName string) (*status.EnvironmentReleaseDiff, error) {
	return r.getReleaseEnvironmentDiff.Do(ctx, fmt.Sprintf("%v###%v", environmentReleaseName, otherEnvironmentReleaseName))
}

func (r *cachingReleaseAccessor) ListEnvironmentReleasesForEnvironment(ctx context.Context, environment string) (*status.EnvironmentReleaseList, error) {
	return r.listEnvironmentReleasesForEnvironment.Do(ctx, environment)
}

func (r *cachingReleaseAccessor) ListEnvironmentReleases(ctx context.Context) (*status.EnvironmentReleaseList, error) {
	return r.listEnvironmentReleases.Do(ctx, "")
}

func (r *cachingReleaseAccessor) SetSelfLookupInstance(accessor ReleaseAccessor) {
	r.selfLookupInstance = accessor
}
