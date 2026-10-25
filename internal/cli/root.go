package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"github.com/sigxcpu/nas/internal/logging"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type RootOptions struct {
	ServerAddr string
	LogLevel   string
	ConfigFile string
	JSONOutput bool
	Logger     *slog.Logger
	Out        io.Writer
	ErrOut     io.Writer
}

func NewRootCmd() *cobra.Command {
	opts := &RootOptions{
		ServerAddr: "127.0.0.1:9090",
		LogLevel:   "info",
		ConfigFile: "",
		Out:        os.Stdout,
		ErrOut:     os.Stderr,
	}

	rootCmd := &cobra.Command{
		Use:   "nasctl",
		Short: "NAS management server and CLI client",
		Long: `nasctl is the unified single-binary daemon and CLI for managing ZFS pools/datasets/volumes,
KVM virtual machines (libvirt + noVNC), Docker & LXC containers, CIFS & WebDAV shares,
and Linux system users/groups.`,
		SilenceUsage: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			errWriter := cmd.ErrOrStderr()
			opts.Out = cmd.OutOrStdout()
			opts.ErrOut = errWriter
			logger := logging.NewLogger(errWriter, logging.ParseLevel(opts.LogLevel))
			slog.SetDefault(logger)
			opts.Logger = logger
		},
	}

	rootCmd.PersistentFlags().StringVarP(&opts.ServerAddr, "server", "s", opts.ServerAddr, "gRPC API server address (host:port) for CLI client commands")
	rootCmd.PersistentFlags().StringVar(&opts.LogLevel, "log-level", opts.LogLevel, "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVarP(&opts.ConfigFile, "config", "c", opts.ConfigFile, "Path to config.yaml configuration file")
	rootCmd.PersistentFlags().BoolVar(&opts.JSONOutput, "json", false, "Output CLI responses as JSON")

	rootCmd.AddCommand(
		newStartCmd(opts),
		newSystemCmd(opts),
		newPrereqCmd(opts),
		newDiskCmd(opts),
		newZfsCmd(opts),
		newVMCmd(opts),
		newDockerCmd(opts),
		newAppCmd(opts),
		newCatalogCmd(opts),
		newLxcCmd(opts),
		newShareCmd(opts),
		newUserCmd(opts),
		newGroupCmd(opts),
	)

	return rootCmd
}

func (o *RootOptions) dialClient(ctx context.Context) (nasv1.NasServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(o.ServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to gRPC server %s: %w", o.ServerAddr, err)
	}
	return nasv1.NewNasServiceClient(conn), conn, nil
}

func (o *RootOptions) printJSON(v any) error {
	enc := json.NewEncoder(o.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
