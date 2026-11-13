package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"

	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"github.com/sigxcpu/nas/internal/logging"
	"github.com/sigxcpu/nas/internal/version"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type RootOptions struct {
	ServerAddr        string
	ServerAddrChanged bool
	TLS               bool
	TLSChanged        bool
	TLSCAFile         string
	TLSInsecure       bool
	Token             string
	LogLevel          string
	ConfigFile        string
	JSONOutput        bool
	Logger            *slog.Logger
	Out               io.Writer
	ErrOut            io.Writer
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
		Use:     "nasctl",
		Version: version.Version,
		Short:   "NAS management server and CLI client",
		Long: `nasctl is the unified single-binary daemon and CLI for managing ZFS pools/datasets/volumes,
KVM virtual machines (libvirt + noVNC), Docker & LXC containers, CIFS & WebDAV shares,
and Linux system users/groups.`,
		SilenceUsage: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			errWriter := cmd.ErrOrStderr()
			opts.Out = cmd.OutOrStdout()
			opts.ErrOut = errWriter
			opts.ServerAddrChanged = cmd.Flags().Changed("server")
			opts.TLSChanged = cmd.Flags().Changed("tls")
			logger := logging.NewLogger(errWriter, logging.ParseLevel(opts.LogLevel))
			slog.SetDefault(logger)
			opts.Logger = logger
		},
	}

	rootCmd.PersistentFlags().StringVarP(&opts.ServerAddr, "server", "s", opts.ServerAddr, "gRPC API server address (host:port) for CLI client commands")
	rootCmd.PersistentFlags().BoolVar(&opts.TLS, "tls", false, "Use TLS when dialing the gRPC server")
	rootCmd.PersistentFlags().StringVar(&opts.TLSCAFile, "tls-ca", "", "Path to PEM CA certificate bundle for gRPC TLS verification")
	rootCmd.PersistentFlags().BoolVar(&opts.TLSInsecure, "tls-insecure", false, "Skip TLS certificate verification when --tls is enabled")
	rootCmd.PersistentFlags().StringVar(&opts.Token, "token", "", "Bearer token for CLI client commands (default: $NASCTL_TOKEN or the token saved by `nasctl login`)")
	rootCmd.PersistentFlags().StringVar(&opts.LogLevel, "log-level", opts.LogLevel, "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVarP(&opts.ConfigFile, "config", "c", opts.ConfigFile, "Path to config.yaml configuration file")
	rootCmd.PersistentFlags().BoolVar(&opts.JSONOutput, "json", false, "Output CLI responses as JSON")

	rootCmd.AddCommand(
		newStartCmd(opts),
		newLoginCmd(opts),
		newLogoutCmd(opts),
		newVersionCmd(opts),
		newUpdateCmd(opts),
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
		newCronCmd(opts),
		newUserCmd(opts),
		newGroupCmd(opts),
	)

	return rootCmd
}

func isLoopbackServerAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		host = strings.TrimSpace(addr)
	}
	if strings.EqualFold(host, "localhost") || host == "" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

func (o *RootOptions) dialClient(ctx context.Context) (nasv1.NasServiceClient, *grpc.ClientConn, error) {
	serverAddr := o.ServerAddr
	useTLS := o.TLS
	tlsCA := o.TLSCAFile
	tlsInsecure := o.TLSInsecure

	if saved, err := loadSavedSession(); err == nil && saved != nil {
		if !o.ServerAddrChanged && saved.ServerAddr != "" {
			serverAddr = saved.ServerAddr
		}
		if !o.TLSChanged && !o.ServerAddrChanged {
			useTLS = saved.TLS
			if tlsCA == "" {
				tlsCA = saved.TLSCAFile
			}
			if !tlsInsecure {
				tlsInsecure = saved.TLSInsecure
			}
		}
	}

	var creds credentials.TransportCredentials
	if useTLS {
		tlsCfg := &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: tlsInsecure,
		}
		if tlsCA != "" {
			pemBytes, err := os.ReadFile(tlsCA)
			if err != nil {
				return nil, nil, fmt.Errorf("reading --tls-ca %q: %w", tlsCA, err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pemBytes) {
				return nil, nil, fmt.Errorf("no valid PEM certificates found in --tls-ca %q", tlsCA)
			}
			tlsCfg.RootCAs = pool
		}
		creds = credentials.NewTLS(tlsCfg)
	} else {
		if !isLoopbackServerAddr(serverAddr) && o.ErrOut != nil {
			_, _ = fmt.Fprintf(o.ErrOut, "warning: dialing non-loopback gRPC server %s without --tls (credentials and tokens are unencrypted)\n", serverAddr)
		}
		creds = insecure.NewCredentials()
	}

	dialOpts := append([]grpc.DialOption{grpc.WithTransportCredentials(creds)}, authDialOptions(o.resolveToken())...)
	conn, err := grpc.NewClient(serverAddr, dialOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to gRPC server %s: %w", serverAddr, err)
	}
	return nasv1.NewNasServiceClient(conn), conn, nil
}

func (o *RootOptions) printJSON(v any) error {
	enc := json.NewEncoder(o.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
