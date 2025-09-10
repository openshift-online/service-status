package release_website

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/aro/client"
	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	release_webserver "github.com/openshift-online/service-status/pkg/aro/release-webserver"
	"github.com/openshift-online/service-status/pkg/util"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
)

type ReleaseMarkdownOptions struct {
	BindAddress net.IP
	BindPort    int

	FileBasedAPIDir string
	AROHCPDir       string
	NumberOfDays    int

	ImageInfoAccessor release_inspection.ImageInfoAccessor
	GitAccessor       release_inspection.ComponentsGitInfo

	util.IOStreams
}

func (o *ReleaseMarkdownOptions) Run(ctx context.Context) error {
	logger := klog.FromContext(ctx)

	releaseAccessor := release_inspection.NewCachingReleaseAccessor(
		release_inspection.NewReleaseAccessor(
			o.AROHCPDir,
			o.NumberOfDays,
			o.ImageInfoAccessor,
			o.GitAccessor,
		),
		clock.RealClock{})

	var releaseClient client.ReleaseClient
	switch {
	case len(o.FileBasedAPIDir) > 0 && len(o.AROHCPDir) > 0:
		return fmt.Errorf("cannot specify both --file-based-api-dir and --aro-hcp-dir")
	case len(o.FileBasedAPIDir) > 0:
		apiFS := os.DirFS(o.FileBasedAPIDir)
		releaseClient = client.NewFileSystemReleaseClient(apiFS)
	case len(o.AROHCPDir) > 0:
		releaseClient = client.NewBasicReleaseClient("http://" + net.JoinHostPort("localhost", fmt.Sprintf("%d", o.BindPort)))
	}

	httpRouter := gin.Default()

	// JSON endpoints
	httpRouter.GET("/api/aro-hcp/environments", release_webserver.ListEnvironments(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/environments/:name", release_webserver.GetEnvironment(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/environments/:name/environmentreleases", release_webserver.ListEnvironmentReleasesForEnvironment(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/environmentreleases", release_webserver.ListEnvironmentReleases(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/environmentreleases/:name", release_webserver.GetEnvironmentRelease(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/environmentreleases/:name/diff/:otherName", release_webserver.GetEnvironmentReleaseDiff(releaseAccessor))

	// HTML endpoints
	httpRouter.LoadHTMLGlob("pkg/aro/release-webserver/html-templates/*")
	httpRouter.GET("", release_webserver.ServeReleaseSummary(releaseClient))
	httpRouter.GET("/http/aro-hcp/summary.html", release_webserver.ServeReleaseSummary(releaseClient))
	httpRouter.GET("/http/aro-hcp/environmentreleases/:name/summary.html", release_webserver.ServeEnvironmentReleaseSummary(releaseClient))

	listener, err := net.Listen("tcp", net.JoinHostPort(o.BindAddress.String(), fmt.Sprintf("%d", o.BindPort)))
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	logger.Info("Starting server", "addr", listener.Addr())
	return httpRouter.RunListener(listener)
}
