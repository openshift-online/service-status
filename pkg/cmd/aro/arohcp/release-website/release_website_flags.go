package release_website

import (
	"context"
	"fmt"
	"net"

	release_inspection "github.com/openshift-online/service-status/pkg/aro/release-inspection"
	"github.com/openshift-online/service-status/pkg/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

// ReleaseMarkdownFlags gets bound to cobra commands and arguments.  It is used to validate input and then produce
// the Options struct.  Options struct is intended to be embeddable and re-useable without cobra.
type ReleaseMarkdownFlags struct {
	BindAddress net.IP
	BindPort    int

	AROHCPDir string

	util.IOStreams
}

func NewReleaseWebsiteCommand(streams util.IOStreams) *cobra.Command {
	f := NewReleaseMarkdownFlags(streams)

	cmd := &cobra.Command{
		Use:           "release-website",
		Short:         "Write markdown summarizing what is in a particular release",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			logger := klog.FromContext(ctx)

			err := f.Validate()
			if err != nil {
				return err
			}

			o, err := f.ToOptions()
			if err != nil {
				return err
			}

			return o.Run(klog.NewContext(context.TODO(), klog.LoggerWithName(logger, "aro hcp release-markdown")))
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}

func NewReleaseMarkdownFlags(streams util.IOStreams) *ReleaseMarkdownFlags {
	return &ReleaseMarkdownFlags{
		IOStreams: streams,
	}
}

func (f *ReleaseMarkdownFlags) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&f.AROHCPDir, "aro-hcp-dir", f.AROHCPDir, "The directory where the https://github.com/Azure/ARO-HCP repo is extracted.")

	flags.IPVar(&f.BindAddress, "bind-address", f.BindAddress, "The IP address on which to listen for the --secure-port port.")
	flags.IntVar(&f.BindPort, "bind-port", f.BindPort, "The port on which to serve HTTP with authentication and authorization.")

}

func (f *ReleaseMarkdownFlags) Validate() error {
	if len(f.AROHCPDir) == 0 {
		return fmt.Errorf("--aro-hcp-dir must be specified")
	}
	if len(f.BindAddress) == 0 || f.BindAddress.IsUnspecified() {
		return fmt.Errorf("--bind-address must be specified")
	}
	if f.BindPort == 0 {
		return fmt.Errorf("--secure-port must be specified")
	}
	return nil
}

func (f *ReleaseMarkdownFlags) ToOptions() (*ReleaseMarkdownOptions, error) {
	return &ReleaseMarkdownOptions{
		BindAddress:       f.BindAddress,
		BindPort:          f.BindPort,
		AROHCPDir:         f.AROHCPDir,
		ImageInfoAccessor: release_inspection.NewThreadSafeImageInfoAccessor(),

		IOStreams: f.IOStreams,
	}, nil
}
