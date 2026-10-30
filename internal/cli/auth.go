package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// tokenFilePath is where `nasctl login` stores the session token for later commands.
func tokenFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "nasctl", "token"), nil
}

// resolveToken picks the session token: --token flag, then $NASCTL_TOKEN, then the saved token file.
func (o *RootOptions) resolveToken() string {
	if o.Token != "" {
		return o.Token
	}
	if t := os.Getenv("NASCTL_TOKEN"); t != "" {
		return t
	}
	if path, err := tokenFilePath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

// authDialOptions attach the bearer token to every outgoing RPC.
func authDialOptions(token string) []grpc.DialOption {
	if token == "" {
		return nil
	}
	withToken := func(ctx context.Context) context.Context {
		return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}
	return []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(withToken(ctx), method, req, reply, cc, opts...)
		}),
		grpc.WithChainStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return streamer(withToken(ctx), desc, cc, method, opts...)
		}),
	}
}

func newLoginCmd(opts *RootOptions) *cobra.Command {
	var username string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Linux (PAM) credentials and save the session token",
		RunE: func(cmd *cobra.Command, args []string) error {
			in := bufio.NewReader(cmd.InOrStdin())
			if username == "" {
				_, _ = fmt.Fprint(opts.ErrOut, "Username: ")
				line, err := in.ReadString('\n')
				if err != nil {
					return fmt.Errorf("reading username: %w", err)
				}
				username = strings.TrimSpace(line)
			}
			_, _ = fmt.Fprint(opts.ErrOut, "Password: ")
			password, err := readPassword(in)
			_, _ = fmt.Fprintln(opts.ErrOut)
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}

			client, conn, err := opts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.Login(cmd.Context(), &nasv1.LoginRequest{Username: username, Password: password})
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}

			path, err := tokenFilePath()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(resp.GetAccessToken()+"\n"), 0o600); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(opts.Out, "Logged in as %s; token saved to %s\n", username, path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "", "Linux username (prompted when omitted)")
	return cmd
}

func newLogoutCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the saved session token",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := tokenFilePath()
			if err != nil {
				return err
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			_, _ = fmt.Fprintln(opts.Out, "Logged out")
			return nil
		},
	}
}

// readPassword reads one line, disabling terminal echo when stdin is a TTY.
func readPassword(in *bufio.Reader) (string, error) {
	fd := int(os.Stdin.Fd())
	if old, err := unix.IoctlGetTermios(fd, unix.TCGETS); err == nil {
		noEcho := *old
		noEcho.Lflag &^= unix.ECHO
		if err := unix.IoctlSetTermios(fd, unix.TCSETS, &noEcho); err == nil {
			defer func() { _ = unix.IoctlSetTermios(fd, unix.TCSETS, old) }()
		}
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
