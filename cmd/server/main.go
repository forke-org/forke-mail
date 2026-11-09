package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"forkemail/internal/config"
	"forkemail/internal/db"
	"forkemail/internal/domains"
	"forkemail/internal/feedback"
	"forkemail/internal/forke"
	"forkemail/internal/ratelimit"
	"forkemail/internal/relay"
	"forkemail/internal/sender"
	"forkemail/internal/ses"
	"forkemail/internal/sns"
	"forkemail/internal/transactional"
	"forkemail/internal/unsub"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
)

const limiterIdleTimeout = 10 * time.Minute

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	var admins *forke.Directory
	if cfg.ForkeDatabaseURL != "" {
		forkePool, err := db.Connect(ctx, cfg.ForkeDatabaseURL)
		if err != nil {
			return fmt.Errorf("forke database: %w", err)
		}
		defer forkePool.Close()
		admins = &forke.Directory{Pool: forkePool}
		visible, err := admins.VisibleAdmins(ctx)
		if err != nil {
			return fmt.Errorf("forke admins table: %w", err)
		}
		if visible == 0 {
			slog.Warn("forke admins table returns no rows for this database user; check row-level security policies, every login will fail")
		}
		slog.Info("forke admins", "visible", visible)
	}

	lim := limiters{public: ratelimit.NewStore(), keys: ratelimit.NewStore(), failures: ratelimit.NewStore()}
	processor := &feedback.Processor{Pool: pool, Reputation: feedback.Thresholds{
		MaxBounceRate: cfg.MaxBounceRate, MaxComplaintRate: cfg.MaxComplaintRate, MinSample: cfg.ReputationMinSample,
	}}
	svc := services{identities: domains.Local{}, feedback: processor, sendingMode: func() string { return "log" }, admins: admins}
	var tasks []func(context.Context)
	developer, err := newDeveloperServices(pool)
	if err != nil {
		return err
	}
	svc.developer = developer
	tasks = append(tasks, developer.Run)
	var mailer sender.Sender = sender.LogSender{Pool: pool, Feedback: processor}
	var account *ses.Account
	if cfg.SMTP() {
		mailer = relay.Sender{Config: cfg.Relay}
		svc.identities = relay.Identities{Allowed: cfg.Relay.AllowedSenders}
		svc.sendingMode = func() string { return "smtp" }
	}
	if cfg.SES() {
		awsCfg, err := ses.LoadConfig(ctx, cfg.AWS.Region)
		if err != nil {
			return err
		}
		client := sesv2.NewFromConfig(awsCfg)
		mailer = ses.Sender{Client: client, ConfigurationSet: cfg.AWS.ConfigurationSet}
		account = &ses.Account{Client: client, Pool: pool}
		identities := ses.Identities{Client: client}
		svc.identities = identities
		svc.verifier = sns.NewVerifier(cfg.AWS.Region, cfg.AWS.TopicARN)
		svc.sendingMode = account.Mode
		tasks = append(tasks, account.Run, func(ctx context.Context) { domains.RunPoller(ctx, pool, identities) })
	}
	worker := sender.NewWorker(pool, mailer, unsub.Signer{Secret: []byte(cfg.UnsubscribeSecret)},
		cfg.PublicBaseURL, cfg.SendRatePerSec, cfg.TrialSendRatePerSec)
	svc.sendRate = worker.GlobalRate
	svc.dispatcher = transactional.NewDispatcher(pool, mailer, worker, transactional.NewTracker([]byte(cfg.UnsubscribeSecret), cfg.PublicBaseURL))
	if cfg.SMTP() {
		quota := relay.Quota{Pool: pool, SendRate: cfg.SendRatePerSec, DailyLimit: cfg.Relay.DailyLimit, MonthlyLimit: cfg.Relay.MonthlyLimit, Apply: worker.SetLimits}
		usage, err := quota.Refresh(ctx)
		if err != nil {
			return fmt.Errorf("smtp quota: %w", err)
		}
		slog.Info("smtp relay", "addr", cfg.Relay.Addr(), "tls", cfg.Relay.TLS, "daily_limit", cfg.Relay.DailyLimit,
			"monthly_limit", cfg.Relay.MonthlyLimit, "sent_24h", usage.Last24Hours, "sent_month", usage.ThisMonth)
		tasks = append(tasks, quota.Run, func(ctx context.Context) { domains.RunPoller(ctx, pool, svc.identities) })
	} else if account == nil {
		if err := worker.SetLimits(sender.Limits{MaxSendRate: cfg.SendRatePerSec}); err != nil {
			return err
		}
	}
	if account != nil {
		account.Apply = worker.SetLimits
		snap, err := account.Refresh(ctx)
		if err != nil {
			return fmt.Errorf("ses get account: %w", err)
		}
		slog.Info("ses account", "region", cfg.AWS.Region, "production", snap.ProductionAccess, "sending_enabled", snap.SendingEnabled,
			"max_send_rate", snap.Limits.MaxSendRate, "max_24h", snap.Limits.Max24HourSend, "sent_24h", snap.Limits.SentLast24Hours)
	}

	var background sync.WaitGroup
	for _, task := range append(tasks,
		worker.Run,
		svc.dispatcher.Run,
		func(ctx context.Context) { sender.RunSweeper(ctx, pool) },
		func(ctx context.Context) { lim.public.Janitor(ctx, limiterIdleTimeout) },
		func(ctx context.Context) { lim.keys.Janitor(ctx, limiterIdleTimeout) },
		func(ctx context.Context) { lim.failures.Janitor(ctx, limiterIdleTimeout) },
	) {
		background.Add(1)
		go func() {
			defer background.Done()
			task(ctx)
		}()
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newRouter(cfg, pool, lim, svc),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	serveErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr, "env", cfg.Env, "sender", cfg.SenderDriver)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		stop()
		background.Wait()
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	background.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
