package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/astraqcd/athena-proxy/internal/control"
	"github.com/astraqcd/athena-proxy/internal/daemon"
	"github.com/astraqcd/athena-proxy/internal/proxy"
	"github.com/astraqcd/athena-proxy/internal/state"
)

func cmdRun(cmd *cobra.Command, controlPort int) error {
	if client, err := connect(); err == nil {
		return fmt.Errorf("a daemon is already running on 127.0.0.1:%d", client.Port())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	d := daemon.New(daemon.Options{
		ControlPort: controlPort,
		Version:     version,
		Out:         out,
		Err:         cmd.ErrOrStderr(),
	})

	if err := d.Start(); err != nil {
		return err
	}
	if err := out.Flush(); err != nil {
		return err
	}
	if len(d.List()) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "no targets registered yet — add one with: athena-proxy add <hostname>")
	}

	<-ctx.Done()
	return d.Close()
}

func cmdAdd(cmd *cobra.Command, hostname, name string, port int) error {
	if _, err := proxy.NormalizeHostname(hostname); err != nil {
		return err
	}

	client, err := connect()
	if err != nil {
		return err
	}

	resp, err := client.Add(control.AddRequest{Hostname: hostname, Name: name, Port: port})
	if err != nil {
		return err
	}

	if resp.Reassigned {
		fmt.Fprintf(cmd.ErrOrStderr(), "local port %d was in use, using %d instead\n", resp.Requested, resp.Target.LocalPort)
	}
	if resp.Existing {
		fmt.Fprintf(cmd.ErrOrStderr(), "already registered\n")
	}
	return printTargets(cmd, []control.Target{resp.Target})
}

func printTargets(cmd *cobra.Command, targets []control.Target) error {
	out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, target := range targets {
		fmt.Fprintf(out, "%s\t%s\n", target.LocalAddr, labelOf(target))
	}
	return out.Flush()
}

func cmdList(cmd *cobra.Command) error {
	client, err := connect()
	if err != nil {
		return err
	}

	resp, err := client.List()
	if err != nil {
		return err
	}
	if len(resp.Targets) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "no targets registered")
		return nil
	}

	return printTargets(cmd, resp.Targets)
}

func cmdRemove(cmd *cobra.Command, selector string) error {
	client, err := connect()
	if err != nil {
		return err
	}

	resp, err := client.Remove(selector)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "removed %s (%s)\n", labelOf(resp.Target), resp.Target.LocalAddr)
	return nil
}

func connect() (*control.Client, error) {
	saved, err := state.Load()
	if err != nil {
		return nil, err
	}
	if saved.ControlPort == 0 {
		return nil, noDaemon()
	}

	client := control.NewClient(saved.ControlPort)
	if _, err := client.Status(); err != nil {
		return nil, noDaemon()
	}
	return client, nil
}

func noDaemon() error {
	return fmt.Errorf("%w (start one with: athena-proxy run)", control.ErrNoDaemon)
}

func labelOf(t control.Target) string {
	if t.Name != "" {
		return t.Name
	}
	return proxy.ShortHostname(t.Hostname)
}
