// Status command: the daemon report operators read.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/server"
)

const (
	statusEndpoint = "/api/v1/status"
	statusTimeout  = 5 * time.Second
)

// ErrNotRunning reports that no daemon answered on the listen port.
var ErrNotRunning = errors.New("relo is not running")

// ErrStatusRefused reports a daemon that answered status with failure.
var ErrStatusRefused = errors.New("management API returned status")

// StatusReport is the daemon state the status command prints.
type StatusReport struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Addr          string `json:"addr"`
	SecretMode    string `json:"secret_mode"`
	SchemaVersion int    `json:"schema_version"`
	ClientKeys    int    `json:"client_keys"`
	ActiveKeys    int    `json:"active_client_keys"`
	// DataPlane names the address of every protocol a client can point at.
	DataPlane []DataPlaneAddress `json:"data_plane"`
	Language  string             `json:"language"`
}

// DataPlaneAddress is one client protocol and the address its listener
// answers on.
type DataPlaneAddress struct {
	Protocol string `json:"protocol"`
	BaseURL  string `json:"base_url"`
}

// Status asks the running daemon for its state over the management
// listener. It reports ErrNotRunning when nothing answers.
func Status(ctx context.Context, home string) (StatusReport, error) {
	cfg, err := config.LoadOffline(home)
	if err != nil {
		return StatusReport{}, err
	}
	token, err := adminToken(home)
	if err != nil {
		return StatusReport{}, err
	}
	return fetchStatus(ctx, cfg.Server.Addr(), token)
}

func adminToken(home string) (string, error) {
	content, err := os.ReadFile(server.TokenPath(home, server.AdminTokenFile))
	if err != nil {
		return "", ErrNotRunning
	}
	token := strings.TrimSpace(string(content))
	if token == "" {
		return "", ErrNotRunning
	}
	return token, nil
}

func fetchStatus(ctx context.Context, addr, token string) (StatusReport, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+statusEndpoint, nil)
	if err != nil {
		return StatusReport{}, fmt.Errorf("build status request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: statusTimeout}
	response, err := client.Do(request)
	if err != nil {
		return StatusReport{}, ErrNotRunning
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return StatusReport{}, fmt.Errorf("%w %d", ErrStatusRefused, response.StatusCode)
	}
	var report StatusReport
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		return StatusReport{}, fmt.Errorf("decode status response: %w", err)
	}
	return report, nil
}

func newStatusCommand(localized ui) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: localized.text("cli.daemon.status.short", nil),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args, localized)
		},
	}
	cmd.Flags().Bool("json", false, localized.text("cli.daemon.status.json", nil))
	return cmd
}

func runStatus(cmd *cobra.Command, _ []string, localized ui) error {
	home, err := homeFromFlags(cmd)
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	report, err := Status(cmd.Context(), home)
	if err != nil {
		return reportStatusFailure(cmd, err, localized)
	}
	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
	}
	printStatus(cmd.OutOrStdout(), report, localized)
	return nil
}

func reportStatusFailure(cmd *cobra.Command, err error, localized ui) error {
	if !errors.Is(err, ErrNotRunning) {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), localized.text("cli.daemon.status.not_running", nil))
	// A daemon that is not running is a distinct outcome, so a script can
	// tell "nothing is up" from "the check itself failed".
	return fail{code: 3, err: err}
}

func printStatus(out io.Writer, report StatusReport, localized ui) {
	_, _ = fmt.Fprintln(out, localized.text("cli.daemon.status.running", nil))
	printStatusLine(out, localized, "cli.daemon.status.version", report.Version)
	printStatusLine(out, localized, "cli.daemon.status.uptime", formatUptime(report.UptimeSeconds))
	printStatusLine(out, localized, "cli.daemon.status.address", report.Addr)
	for index, address := range report.DataPlane {
		label := ""
		if index == 0 {
			label = "cli.daemon.status.data_plane"
		}
		printStatusLine(out, localized, label, address.Protocol+"  "+address.BaseURL)
	}
	printStatusLine(out, localized, "cli.daemon.status.secret_mode", report.SecretMode)
	printStatusLine(out, localized, "cli.daemon.status.schema_version", strconv.Itoa(report.SchemaVersion))
	printStatusLine(out, localized, "cli.daemon.status.client_keys", localized.text(
		"cli.daemon.status.key_count", map[string]any{"Count": report.ClientKeys, "Active": report.ActiveKeys}))
	if report.ActiveKeys == 0 {
		_, _ = fmt.Fprintln(out, "\n"+localized.text("cli.daemon.status.no_keys", nil))
		_, _ = fmt.Fprintln(out, localized.text("cli.daemon.status.no_keys_hint", nil))
	}
}

func printStatusLine(out io.Writer, localized ui, label, value string) {
	name := ""
	if label != "" {
		name = localized.text(label, nil) + ":"
	}
	_, _ = fmt.Fprintf(out, "  %-16s %s\n", name, value)
}

func formatUptime(seconds int64) string {
	switch {
	case seconds >= 3600:
		return fmt.Sprintf("%dh %dm", seconds/3600, (seconds%3600)/60)
	case seconds >= 60:
		return fmt.Sprintf("%dm", seconds/60)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}
