// Command vetod is the Veto server. It serves the agent API, the console API
// and the MCP proxy, runs background jobs, and provides operator commands.
//
// Usage:
//
//	vetod [serve]                 run the server (the default)
//	vetod migrate                 apply database migrations and exit
//	vetod keygen                  print a new master key
//	vetod vault rekey             reseal secrets under the current master key
//	vetod ledger verify           verify every workspace's audit ledger
//	vetod eval -file cases.jsonl  score question packs against labelled cases
//	vetod healthcheck             exit 0 if the local server is healthy
//	vetod version                 print the version
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/config"
	"github.com/shvgroups/veto/server/internal/connector"
	"github.com/shvgroups/veto/server/internal/decide"
	"github.com/shvgroups/veto/server/internal/eval"
	"github.com/shvgroups/veto/server/internal/events"
	"github.com/shvgroups/veto/server/internal/httpapi"
	"github.com/shvgroups/veto/server/internal/jev"
	"github.com/shvgroups/veto/server/internal/mcp"
	"github.com/shvgroups/veto/server/internal/metrics"
	"github.com/shvgroups/veto/server/internal/notify"
	"github.com/shvgroups/veto/server/internal/store"
	"github.com/shvgroups/veto/server/internal/vault"
	"github.com/shvgroups/veto/server/internal/worker"
)

func main() {
	// Timestamps in API responses and logs are UTC whatever the host's zone.
	time.Local = time.UTC
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vetod:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return serve()
	case "migrate":
		return migrate()
	case "keygen":
		fmt.Println(vault.GenerateKey())
		return nil
	case "vault":
		if len(args) == 1 && args[0] == "rekey" {
			return rekey()
		}
		return errors.New("usage: vetod vault rekey")
	case "ledger":
		if len(args) == 1 && args[0] == "verify" {
			return verifyLedgers()
		}
		return errors.New("usage: vetod ledger verify")
	case "eval":
		return runEval(args)
	case "healthcheck":
		return healthcheck()
	case "version", "-v", "--version":
		fmt.Println("vetod", app.Version)
		return nil
	case "help", "-h", "--help":
		fmt.Println(strings.TrimSpace(usage))
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, strings.TrimSpace(usage))
}

const usage = `
usage: vetod <command>

commands:
  serve        run the server (default)
  migrate      apply database migrations and exit
  keygen       print a new master key for VETO_MASTER_KEY
  vault rekey  reseal secrets under the current master key
  ledger verify  verify every workspace's audit ledger
  eval         score question packs against labelled cases (vetod eval -h)
  healthcheck  exit 0 if the local server answers /healthz
  version      print the version
`

func newLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	var h slog.Handler = slog.NewJSONHandler(os.Stderr, opts)
	if cfg.LogFormat == "text" {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h).With("service", "vetod")
}

// deps opens the store and keyring and builds the app. The returned cleanup
// closes the store.
type deps struct {
	cfg     *config.Config
	log     *slog.Logger
	store   *store.Store
	keyring *vault.Keyring
	app     *app.App
	bus     *events.Bus
	reg     *metrics.Registry
}

func open(ctx context.Context) (*deps, error) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return nil, err
	}
	log := newLogger(cfg)
	s, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	kr, err := vault.NewKeyring(cfg.MasterKey, cfg.PreviousMasterKey...)
	if err != nil {
		s.Close()
		return nil, err
	}
	reg := metrics.NewRegistry()
	bus := events.NewBus(s.Pool(), log)

	d := app.Deps{
		Store: s, Bus: bus, Keyring: kr, Log: log, Metrics: app.NewMetrics(reg),
		Slack:           notify.NewSlack(&http.Client{Timeout: 10 * time.Second}, notify.SlackAPI),
		Connector:       app.HTTPConnector(connector.Client(false, 30*time.Second)),
		PublicURL:       cfg.PublicURL,
		DecisionTimeout: cfg.DecisionTimeout,
		SessionTTL:      cfg.SessionTTL,
		MaxWait:         cfg.MaxWait,
	}
	if !cfg.ConnectorPublic {
		d.PrivateConnector = app.HTTPConnector(connector.Client(true, 30*time.Second))
	}
	if cfg.ModelConfigured() {
		client := jev.NewClient(cfg.Jev)
		d.Model, d.ModelName = client, client.PrimaryModel()
	}
	a, err := app.New(d)
	if err != nil {
		s.Close()
		return nil, err
	}
	return &deps{cfg: cfg, log: log, store: s, keyring: kr, app: a, bus: bus, reg: reg}, nil
}

func serve() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	d, err := open(ctx)
	if err != nil {
		return err
	}
	defer d.store.Close()
	log, cfg := d.log, d.cfg

	if os.Getenv("VETO_AUTO_MIGRATE") != "false" {
		ran, err := d.store.Migrate(ctx)
		if err != nil {
			return err
		}
		if len(ran) > 0 {
			log.Info("applied migrations", "versions", ran)
		}
	}

	// Background work gets its own context so it can be stopped, and its
	// connections released, before the store closes.
	bgCtx, bgCancel := context.WithCancel(context.Background())
	bgDone := make(chan struct{})
	go func() {
		defer close(bgDone)
		done := make(chan struct{})
		go func() { d.bus.Run(bgCtx); close(done) }()
		if cfg.RunWorkers {
			sender := notify.NewSender(connector.Client(!cfg.ConnectorPublic, notify.DeliveryTimeout))
			worker.New(d.app, d.store, sender, log, worker.DefaultIntervals(), d.reg).Run(bgCtx)
		}
		<-done
	}()

	upstream := connector.Transport(!cfg.ConnectorPublic)
	upstream.ResponseHeaderTimeout = 0 // MCP responses may stream for a long time.
	proxy := mcp.New(d.app, &http.Client{Transport: upstream, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, cfg.MCPHoldTimeout, log)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpapi.New(d.app, cfg, log, proxy, d.reg).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("vetod listening", "addr", cfg.ListenAddr, "version", app.Version,
			"model", d.app.ModelName, "model_configured", d.app.ModelConfigured(), "workers", cfg.RunWorkers)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		bgCancel()
		<-bgDone
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown did not finish cleanly", "error", err)
	}
	bgCancel()
	<-bgDone
	log.Info("stopped")
	return nil
}

func migrate() error {
	ctx := context.Background()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	s, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer s.Close()
	ran, err := s.Migrate(ctx)
	if err != nil {
		return err
	}
	if len(ran) == 0 {
		fmt.Println("database is up to date")
	}
	for _, v := range ran {
		fmt.Println("applied", v)
	}
	return nil
}

func rekey() error {
	ctx := context.Background()
	d, err := open(ctx)
	if err != nil {
		return err
	}
	defer d.store.Close()
	rep, err := d.app.Rekey(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("resealed %d secrets (signing keys %d, connections %d, webhooks %d, integrations %d)\n",
		rep.Total(), rep.SigningKeys, rep.Connections, rep.Webhooks, rep.Integrations)
	return nil
}

func verifyLedgers() error {
	ctx := context.Background()
	d, err := open(ctx)
	if err != nil {
		return err
	}
	defer d.store.Close()
	ids, err := d.store.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	failed := 0
	for _, ws := range ids {
		rep, err := d.app.VerifyWorkspaceLedger(ctx, ws)
		if err != nil {
			return err
		}
		if rep.Break != nil {
			failed++
			fmt.Printf("%s  BROKEN at entry %d: %s\n", ws, rep.Break.Seq, rep.Break.Reason)
			continue
		}
		fmt.Printf("%s  ok  %d entries  head %s\n", ws, rep.Entries, rep.HeadHash)
	}
	if failed > 0 {
		return fmt.Errorf("%d workspace ledgers failed verification", failed)
	}
	return nil
}

func runEval(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	file := fs.String("file", "", "JSON Lines file of labelled cases")
	jsonOut := fs.Bool("json", false, "print the full report as JSON")
	order := fs.Bool("order-check", true, "ask each disposition twice with options reversed")
	timeout := fs.Duration("timeout", 10*time.Second, "time limit per case")
	packDir := fs.String("packs", "", "directory of question pack JSON files that replace or add to the built-in packs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("eval needs -file")
	}
	cfg, err := config.Load(func(k string) string {
		// eval does not touch the database or the vault.
		switch k {
		case "VETO_DATABASE_URL":
			return "postgres://unused"
		case "VETO_MASTER_KEY":
			if v := os.Getenv(k); v != "" {
				return v
			}
			return vault.GenerateKey()
		}
		return os.Getenv(k)
	})
	if err != nil {
		return err
	}
	if !cfg.ModelConfigured() {
		return errors.New("eval needs a decision model: set VETO_JEV_API_KEY")
	}
	f, err := os.Open(*file)
	if err != nil {
		return err
	}
	defer f.Close()
	cases, err := eval.Load(f)
	if err != nil {
		return err
	}
	packs, err := decide.Builtin()
	if err != nil {
		return err
	}
	if *packDir != "" {
		if err := loadPacks(*packDir, packs); err != nil {
			return err
		}
	}
	rep := eval.Run(context.Background(), jev.NewClient(cfg.Jev), packs, cases, *order, *timeout)
	if *jsonOut {
		return writeJSON(os.Stdout, rep)
	}
	eval.WriteText(os.Stdout, rep)
	if rep.Unsafe > 0 {
		return fmt.Errorf("%d cases would have run unsafely without a person", rep.Unsafe)
	}
	return nil
}

func healthcheck() error {
	addr := os.Getenv("VETO_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
		if p := os.Getenv("PORT"); p != "" {
			addr = ":" + p
		}
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + addr + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: HTTP %d", resp.StatusCode)
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// loadPacks reads every .json file in dir as a question pack and adds it to
// packs, replacing a built-in pack of the same name.
func loadPacks(dir string, packs map[string]*decide.Pack) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no .json files in %s", dir)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		p, err := decide.ParsePack(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		packs[p.Name] = p
	}
	return nil
}
