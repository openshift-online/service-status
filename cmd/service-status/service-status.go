package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/openshift-online/service-status/pkg/cmd/aro"
	"github.com/openshift-online/service-status/pkg/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

func main() {
	klog.EnableContextualLogging(true)

	root := &cobra.Command{
		Long: `Show the status of our service.`,
	}

	streams := util.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	root.AddCommand(
		aro.NewAROCommand(streams),
	)

	// TODO re-enable if we use klog
	klog.InitFlags(flag.CommandLine)
	f := flag.CommandLine.Lookup("v")
	root.PersistentFlags().AddGoFlag(f)
	pflag.CommandLine = pflag.NewFlagSet("empty", pflag.ExitOnError)
	flag.CommandLine = flag.NewFlagSet("empty", flag.ExitOnError)

	if err := func() error {
		return root.Execute()
	}(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

}
