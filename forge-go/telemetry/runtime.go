package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rustic-ai/forge/forge-go/forgepath"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const (
	TelemetryModeDesktopSQLite = "desktop_sqlite"
	TelemetryModeExternalOTLP  = "external_otlp"
)

type Config struct {
	Enabled          bool
	Mode             string
	EndpointURL      string
	ServiceName      string
	ServiceVersion   string
	SQLiteBinaryPath string
	SQLiteDBPath     string
	SQLitePort       int
}

type Runtime struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	sidecar        *sqliteOTelSidecar
	startupCounter metricapi.Int64Counter
	mode           string
}

func Start(ctx context.Context, cfg Config) (*Runtime, error) {
	if !cfg.Enabled {
		return &Runtime{}, nil
	}

	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	rt := &Runtime{mode: cfg.Mode}
	if cfg.Mode == TelemetryModeDesktopSQLite {
		sidecar, err := startSQLiteOTel(ctx, cfg)
		if err != nil {
			return nil, err
		}
		rt.sidecar = sidecar
		cfg.EndpointURL = sidecar.endpointURL
	}

	tracerProvider, meterProvider, err := initProviders(ctx, cfg)
	if err != nil {
		_ = rt.stopSidecar(context.Background())
		return nil, err
	}
	rt.tracerProvider = tracerProvider
	rt.meterProvider = meterProvider
	meter := meterProvider.Meter("forge.telemetry")
	rt.startupCounter, _ = meter.Int64Counter("forge.telemetry.startups")
	rt.startupCounter.Add(ctx, 1, metricapi.WithAttributes(attribute.String("forge.telemetry.mode", cfg.Mode)))
	return rt, nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	var errs []error
	if r.meterProvider != nil {
		if err := r.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if r.tracerProvider != nil {
		if err := r.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if err := r.stopSidecar(ctx); err != nil {
		errs = append(errs, err)
	}

	otel.SetTracerProvider(tracenoop.NewTracerProvider())
	resetMetricProvider()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return errors.Join(errs...)
}

func (r *Runtime) stopSidecar(ctx context.Context) error {
	if r.sidecar == nil {
		return nil
	}
	return r.sidecar.stop(ctx)
}

func (c *Config) normalize() {
	c.Mode = strings.TrimSpace(strings.ToLower(c.Mode))
	if c.Mode == "" {
		c.Mode = TelemetryModeDesktopSQLite
	}
	c.ServiceName = strings.TrimSpace(c.ServiceName)
	if c.ServiceName == "" {
		c.ServiceName = "forge-server"
	}
	c.EndpointURL = strings.TrimSpace(c.EndpointURL)
	c.SQLiteBinaryPath = strings.TrimSpace(c.SQLiteBinaryPath)
	if c.SQLitePort == 0 {
		c.SQLitePort = 4318
	}
	if strings.TrimSpace(c.SQLiteDBPath) == "" {
		c.SQLiteDBPath = forgeSQLiteDefaultDBPath()
	}
}

func (c Config) validate() error {
	switch c.Mode {
	case TelemetryModeDesktopSQLite:
		if c.SQLiteBinaryPath == "" {
			return fmt.Errorf("telemetry mode %q requires a sqlite-otel binary path", c.Mode)
		}
	case TelemetryModeExternalOTLP:
		if c.EndpointURL == "" {
			return fmt.Errorf("telemetry mode %q requires an OTLP endpoint URL", c.Mode)
		}
	default:
		return fmt.Errorf("unsupported telemetry mode %q", c.Mode)
	}

	if c.EndpointURL != "" {
		if _, err := parseEndpointURL(c.EndpointURL); err != nil {
			return err
		}
	}
	return nil
}

func forgeSQLiteDefaultDBPath() string {
	return filepath.Join(forgepath.Resolve("telemetry"), "sqlite-otel.db")
}

type sqliteOTelSidecar struct {
	cmd         *exec.Cmd
	waitCh      chan error
	endpointURL string
}

func startSQLiteOTel(ctx context.Context, cfg Config) (*sqliteOTelSidecar, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.SQLiteDBPath), 0o755); err != nil {
		return nil, fmt.Errorf("create telemetry db directory: %w", err)
	}

	cmd := exec.Command(cfg.SQLiteBinaryPath,
		"-port", strconv.Itoa(cfg.SQLitePort),
		"-db-path", cfg.SQLiteDBPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sqlite-otel: %w", err)
	}

	sidecar := &sqliteOTelSidecar{
		cmd:         cmd,
		waitCh:      make(chan error, 1),
		endpointURL: fmt.Sprintf("http://127.0.0.1:%d", cfg.SQLitePort),
	}
	go func() {
		sidecar.waitCh <- cmd.Wait()
	}()

	if err := waitForCollectorReady(ctx, cfg.SQLitePort, sidecar.waitCh); err != nil {
		_ = sidecar.stop(context.Background())
		return nil, err
	}

	return sidecar, nil
}

func waitForCollectorReady(ctx context.Context, port int, waitCh <-chan error) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-waitCh:
			return fmt.Errorf("sqlite-otel exited before becoming ready: %w", err)
		default:
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for sqlite-otel on %s", address)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *sqliteOTelSidecar) stop(ctx context.Context) error {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	_ = s.cmd.Process.Signal(os.Interrupt)
	select {
	case err := <-s.waitCh:
		if err == nil || errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == -1 {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}

	if err := s.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case err := <-s.waitCh:
		if err == nil || errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	case <-time.After(2 * time.Second):
		return fmt.Errorf("sqlite-otel did not exit after kill")
	}
}

func parseEndpointURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse OTLP endpoint URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("OTLP endpoint URL must include scheme and host: %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("OTLP endpoint URL must use http or https: %q", raw)
	}
	return parsed, nil
}
