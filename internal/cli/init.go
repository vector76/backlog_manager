package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vector76/backlog_manager/internal/config"
)

func newInitCmd() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Interactively create a server config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(outputPath); err == nil {
				return fmt.Errorf("%s already exists; delete it first or use --output to choose a different path", outputPath)
			}

			r := bufio.NewReader(cmd.InOrStdin())
			w := cmd.OutOrStdout()

			port, err := initPromptPort(r, w)
			if err != nil {
				return err
			}
			dataDir := initPromptDataDir(r, w)
			user := initPromptUser(r, w)
			password, err := initPromptPassword(r, w)
			if err != nil {
				return err
			}
			beadsURL := initPromptBeadsURL(r, w)
			viewerUser := initPromptViewerUser(r, w)
			viewerPassword := initPromptViewerPassword(r, w)

			cfg := config.Config{
				Port:              port,
				DataDir:           dataDir,
				DashboardUser:     user,
				DashboardPassword: password,
				BeadsServerURL:    beadsURL,
				ViewerUser:        viewerUser,
				ViewerPassword:    viewerPassword,
			}

			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}

			if err := os.WriteFile(outputPath, data, 0600); err != nil {
				return fmt.Errorf("write config: %w", err)
			}

			fmt.Fprintf(w, "Config written to %s\n", outputPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&outputPath, "output", "config.json", "path to write config file")
	return cmd
}

// initReadLine reads a line from r, trimming whitespace.
// Returns the trimmed content and nil error if any non-whitespace content was read.
// Returns ("", err) when the trimmed line is empty (blank line or EOF).
func initReadLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	s := strings.TrimSpace(line)
	if s != "" {
		return s, nil
	}
	return "", err
}

func initPromptPort(r *bufio.Reader, w io.Writer) (int, error) {
	fmt.Fprint(w, "Port [8080]: ")
	s, _ := initReadLine(r)
	if s == "" {
		return 8080, nil
	}
	p, err := strconv.Atoi(s)
	if err != nil || p <= 0 || p > 65535 {
		return 0, fmt.Errorf("invalid port: %q", s)
	}
	return p, nil
}

func initPromptDataDir(r *bufio.Reader, w io.Writer) string {
	def := initDefaultDataDir()
	fmt.Fprintf(w, "Data directory [%s]: ", def)
	s, _ := initReadLine(r)
	if s == "" {
		return def
	}
	return s
}

func initPromptUser(r *bufio.Reader, w io.Writer) string {
	fmt.Fprint(w, "Dashboard user [admin]: ")
	s, _ := initReadLine(r)
	if s == "" {
		return "admin"
	}
	return s
}

func initPromptPassword(r *bufio.Reader, w io.Writer) (string, error) {
	for {
		fmt.Fprint(w, "Dashboard password: ")
		s, err := initReadLine(r)
		if s != "" {
			return s, nil
		}
		if err != nil {
			return "", fmt.Errorf("dashboard password is required")
		}
		fmt.Fprintln(w, "Password is required.")
	}
}

func initPromptBeadsURL(r *bufio.Reader, w io.Writer) string {
	fmt.Fprint(w, "Beads Server URL (optional, press Enter to skip): ")
	s, _ := initReadLine(r)
	return s
}

func initPromptViewerUser(r *bufio.Reader, w io.Writer) string {
	fmt.Fprint(w, "Viewer user (optional, press Enter to disable): ")
	s, _ := initReadLine(r)
	return s
}

func initPromptViewerPassword(r *bufio.Reader, w io.Writer) string {
	fmt.Fprint(w, "Viewer password (optional, press Enter to skip): ")
	s, _ := initReadLine(r)
	return s
}

func initDefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./bm-data"
	}
	return filepath.Join(home, ".local", "share", "bm")
}
