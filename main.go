package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "athena-proxy",
		Short:         "Local TCP tunnels for Athena CTF challenge instances",
		Long:          "athena-proxy gives a plain TCP address on localhost for every challenge hostname you register, and speaks TLS and SNI to the gateway on your tooling's behalf.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Example: `
  athena-proxy run &
  athena-proxy add s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz --name pwn-heap
  nc 127.0.0.1 13370
`,
	}

	root.AddCommand(newRunCmd(), newAddCmd(), newListCmd(), newRemoveCmd())
	return root
}

func newRunCmd() *cobra.Command {
	var controlPort int
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start the daemon and serve every registered target",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdRun(cmd, controlPort)
		},
	}
	cmd.Flags().IntVar(&controlPort, "control-port", 0, "Loopback port for the control API (default: an ephemeral port)")
	return cmd
}

func newAddCmd() *cobra.Command {
	var name string
	var port int
	cmd := &cobra.Command{
		Use:   "add <hostname>",
		Short: "Register a challenge hostname and print its local address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdAdd(cmd, args[0], name, port)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Label to show instead of the hostname")
	cmd.Flags().IntVar(&port, "port", 0, "Pin the local port instead of taking the next free one")
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show every registered target and its local address",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdList(cmd)
		},
	}
}

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name|hostname|local-port>",
		Aliases: []string{"rm"},
		Short:   "Drop a target and close its listener",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdRemove(cmd, args[0])
		},
	}
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "athena-proxy: %v\n", err)
		os.Exit(1)
	}
}
