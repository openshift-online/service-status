package aro

import (
	"fmt"

	"github.com/openshift-online/service-status/pkg/cmd/aro/arohcp"
	"github.com/openshift-online/service-status/pkg/util"

	"github.com/spf13/cobra"
)

func NewAROCommand(streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "aro",
		Short:         "",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("subcommand only")
		},
	}

	cmd.AddCommand(
		arohcp.NewHCPCommand(streams),
	)

	return cmd
}
