package arohcp

import (
	"fmt"

	release_website "github.com/openshift-online/service-status/pkg/cmd/aro/arohcp/release-website"
	"github.com/openshift-online/service-status/pkg/util"

	release_markdown "github.com/openshift-online/service-status/pkg/cmd/aro/arohcp/release-markdown"
	"github.com/spf13/cobra"
)

func NewHCPCommand(streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "hcp",
		Short:         "",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("subcommand only")
		},
	}

	cmd.AddCommand(
		release_markdown.NewReleaseMarkdownCommand(streams),
		release_website.NewReleaseWebsiteCommand(streams),
	)

	return cmd
}
