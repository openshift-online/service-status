package release_website

import (
	"context"
	"fmt"
	"net"

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

	AROHCPDir string

	ImageInfoAccessor release_inspection.ImageInfoAccessor

	util.IOStreams
}

func (o *ReleaseMarkdownOptions) Run(ctx context.Context) error {
	logger := klog.FromContext(ctx)

	releaseAccessor := release_webserver.NewCachingReleaseAccessor(
		release_webserver.NewReleaseAccessor(o.AROHCPDir, o.ImageInfoAccessor),
		clock.RealClock{})
	releaseClient := client.NewBasicReleaseClient("http://" + net.JoinHostPort("localhost", fmt.Sprintf("%d", o.BindPort)))

	httpRouter := gin.Default()

	// JSON endpoints
	httpRouter.GET("/api/aro-hcp/environments", release_webserver.ListEnvironments(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/environments/:name", release_webserver.GetEnvironment(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/releases", release_webserver.ListReleases(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/releases/:name", release_webserver.GetRelease(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/environmentreleases", release_webserver.ListEnvironmentReleases(releaseAccessor))
	httpRouter.GET("/api/aro-hcp/environmentreleases/:name", release_webserver.GetEnvironmentRelease(releaseAccessor))

	// HTML endpoints
	httpRouter.LoadHTMLGlob("pkg/aro/release-webserver/html-templates/*")
	httpRouter.GET("/http/aro-hcp/summary.html", release_webserver.ServeReleaseSummary(releaseClient))

	listener, err := net.Listen("tcp", net.JoinHostPort(o.BindAddress.String(), fmt.Sprintf("%d", o.BindPort)))
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	logger.Info("Starting server", "addr", listener.Addr())
	return httpRouter.RunListener(listener)
}
