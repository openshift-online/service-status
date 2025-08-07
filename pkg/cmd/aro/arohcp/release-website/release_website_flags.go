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

	FileBasedAPIDir           string
	AROHCPDir                 string
	PullSecretDir             string
	ComponentGitRepoParentDir string
	NumberOfDays              int

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
		IOStreams:    streams,
		NumberOfDays: 14,
	}
}

func (f *ReleaseMarkdownFlags) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&f.FileBasedAPIDir, "filebased-api-dir", f.FileBasedAPIDir, "The directory to read canned responses.")
	flags.StringVar(&f.AROHCPDir, "aro-hcp-dir", f.AROHCPDir, "The directory where the https://github.com/Azure/ARO-HCP repo is extracted.")
	flags.StringVar(&f.PullSecretDir, "pull-secret-dir", f.PullSecretDir, "The directory where dockerconfig.json's are located.")
	flags.StringVar(&f.ComponentGitRepoParentDir, "component-git-repo-storage-dir", f.ComponentGitRepoParentDir, "The parent directory where components will be extracted for diff analysis.")

	flags.IPVar(&f.BindAddress, "bind-address", f.BindAddress, "The IP address on which to listen for the --secure-port port.")
	flags.IntVar(&f.BindPort, "bind-port", f.BindPort, "The port on which to serve HTTP with authentication and authorization.")

	flags.IntVar(&f.NumberOfDays, "num-days", f.NumberOfDays, "The number of days to look back for releases.")

}

func (f *ReleaseMarkdownFlags) Validate() error {
	switch {
	case len(f.AROHCPDir) > 0 && len(f.FileBasedAPIDir) > 0:
		return fmt.Errorf("only one of --filebased-api-dir and --aro-hcp-dir can be specified")
	case len(f.AROHCPDir) == 0 && len(f.FileBasedAPIDir) == 0:
		return fmt.Errorf("one of --filebased-api-dir and --aro-hcp-dir must be specified")
	}

	if len(f.PullSecretDir) == 0 {
		return fmt.Errorf("--pull-secret-dir must be specified")
	}
	if len(f.AROHCPDir) > 0 {
		return nil
	}
	if len(f.FileBasedAPIDir) > 0 {
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
	gitAccessor := release_inspection.NewDummyComponentsGitInfo()
	if len(f.ComponentGitRepoParentDir) > 0 {
		gitAccessor = release_inspection.NewComponentsGitInfo(f.ComponentGitRepoParentDir)
	}

	return &ReleaseMarkdownOptions{
		BindAddress:       f.BindAddress,
		BindPort:          f.BindPort,
		FileBasedAPIDir:   f.FileBasedAPIDir,
		AROHCPDir:         f.AROHCPDir,
		NumberOfDays:      f.NumberOfDays,
		ImageInfoAccessor: release_inspection.NewThreadSafeImageInfoAccessor(f.PullSecretDir),
		GitAccessor:       gitAccessor,

		IOStreams: f.IOStreams,
	}, nil
}
