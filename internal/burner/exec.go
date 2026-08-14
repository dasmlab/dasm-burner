package burner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

func FindBinary() (string, error) {
	if p := os.Getenv("KUBE_BURNER"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("kube-burner"); err == nil {
		return p, nil
	}
	candidates := []string{
		filepath.Join("bin", "kube-burner"),
		filepath.Join(".", "bin", "kube-burner"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "kube-burner"))
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs, nil
			}
		}
	}
	return "", fmt.Errorf("kube-burner %s not found; put it on PATH or in ./bin (KUBE_BURNER=...)", KubeBurnerVersion)
}

type MeasureProc struct {
	Cmd *exec.Cmd
}

func StartMeasure(ctx context.Context, bin, measureYml, kubeconfig, runID, userMeta string, duration time.Duration) (*MeasureProc, error) {
	if duration <= 0 {
		duration = 10 * time.Minute
	}
	args := []string{
		"measure",
		"-c", measureYml,
		"--duration", duration.String(),
		"--selector", topology.Selector(runID),
		"--uuid", runID,
		"--job-name", "dasm-burner-" + runID,
		"--skip-log-file",
		"--log-level", "info",
	}
	if userMeta != "" {
		args = append(args, "--user-metadata", userMeta)
	}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &MeasureProc{Cmd: cmd}, nil
}

func (m *MeasureProc) Wait() error {
	if m == nil || m.Cmd == nil {
		return nil
	}
	return ignoreMeasureExit(m.Cmd.Wait())
}

func (m *MeasureProc) Stop() error {
	if m == nil || m.Cmd == nil || m.Cmd.Process == nil {
		return nil
	}
	_ = m.Cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- m.Cmd.Wait() }()
	select {
	case err := <-done:
		return ignoreMeasureExit(err)
	case <-time.After(15 * time.Second):
		_ = m.Cmd.Process.Kill()
		return ignoreMeasureExit(<-done)
	}
}

func ignoreMeasureExit(err error) error {
	if err == nil {
		return nil
	}
	s := err.Error()
	if s == "signal: terminated" || s == "signal: killed" {
		return nil
	}
	return err
}

func Index(ctx context.Context, bin, kubeconfig, promURL, tokenFile, metricsYml, metricsDir, uuid, userMeta string, start, end time.Time) error {
	args := []string{
		"index",
		"--uuid", uuid,
		"--prometheus-url", promURL,
		"--token-file", tokenFile,
		"--metrics-profile", metricsYml,
		"--metrics-directory", metricsDir,
		"--indexer-type", "local",
		"--tarball-name", MetricsTarballName,
		"--start", strconv.FormatInt(start.Unix(), 10),
		"--end", strconv.FormatInt(end.Unix(), 10),
		"--skip-tls-verify",
		"--job-name", "dasm-burner-" + uuid,
		"--skip-log-file",
	}
	if userMeta != "" {
		args = append(args, "--user-metadata", userMeta)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if kubeconfig != "" {
		cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
	}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func CheckAlerts(ctx context.Context, bin, promURL, tokenFile, alertsYml, metricsDir, uuid string, start, end time.Time) error {
	args := []string{
		"check-alerts",
		"--uuid", uuid,
		"--prometheus-url", promURL,
		"--token-file", tokenFile,
		"--alert-profile", alertsYml,
		"--metrics-directory", metricsDir,
		"--start", strconv.FormatInt(start.Unix(), 10),
		"--end", strconv.FormatInt(end.Unix(), 10),
		"--skip-tls-verify",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunInit executes kube-burner init for ObjectPressure creates.
func RunInit(ctx context.Context, bin, initYml, kubeconfig, runID string) error {
	args := []string{
		"init",
		"-c", initYml,
		"--uuid", runID,
		"--skip-log-file",
		"--log-level", "info",
	}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
