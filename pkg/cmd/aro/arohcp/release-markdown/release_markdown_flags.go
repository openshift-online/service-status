package release_markdown

import (
	"context"
	"fmt"

	"github.com/openshift-online/service-status/pkg/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

// ReleaseMarkdownFlags gets bound to cobra commands and arguments.  It is used to validate input and then produce
// the Options struct.  Options struct is intended to be embeddable and re-useable without cobra.
type ReleaseMarkdownFlags struct {
	AROHCPDir string
	OutputDir string

	util.IOStreams
}

func NewReleaseMarkdownCommand(streams util.IOStreams) *cobra.Command {
	f := NewReleaseMarkdownFlags(streams)

	cmd := &cobra.Command{
		Use:           "release-markdown",
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
	flags.StringVar(&f.OutputDir, "output-dir", f.OutputDir, "The directory where the https://github.com/Azure/ARO-HCP repo is extracted.")
}

func (f *ReleaseMarkdownFlags) Validate() error {
	if len(f.AROHCPDir) == 0 {
		return fmt.Errorf("--aro-hcp-dir must be specified")
	}
	if len(f.OutputDir) == 0 {
		return fmt.Errorf("--output-dir must be specified")
	}
	return nil
}

func (f *ReleaseMarkdownFlags) ToOptions() (*ReleaseMarkdownOptions, error) {
	return &ReleaseMarkdownOptions{
		AROHCPDir:         f.AROHCPDir,
		OutputDir:         f.OutputDir,
		ImageInfoAccessor: newThreadSafeImageInfoAccessor(),

		IOStreams: f.IOStreams,
	}, nil
}
