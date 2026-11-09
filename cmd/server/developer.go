package main

import (
	"context"
	"errors"
	"forkemail/internal/transactional"
	"os"

	"forkemail/internal/webhooks"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type developerServices struct {
	transaction *transactional.CompatibilityHandler
	hooks       *webhooks.Handler
	dispatcher  *webhooks.Dispatcher
}

func newDeveloperServices(pool *pgxpool.Pool, development bool) (*developerServices, error) {
	h := &webhooks.Handler{Pool: pool}
	d := &developerServices{hooks: h}
	d.transaction = &transactional.CompatibilityHandler{Pool: pool}
	if os.Getenv("TRANSACTIONAL_STUB_ENABLED") == "true" {
		if !development {
			return nil, errors.New("TRANSACTIONAL_STUB_ENABLED is restricted to development")
		}
		d.transaction.Enqueuer = transactional.DevelopmentStub{}
		d.transaction.Development = true
	}
	if encoded := os.Getenv("WEBHOOK_ENCRYPTION_KEY"); encoded != "" {
		v, err := webhooks.NewVault(encoded)
		if err != nil {
			return nil, err
		}
		h.Vault = v
		d.dispatcher = &webhooks.Dispatcher{Pool: pool, Vault: v}
	}
	return d, nil
}

func (d *developerServices) Run(ctx context.Context) {
	if d.dispatcher != nil {
		d.dispatcher.Run(ctx)
	}
}

func registerDeveloperRoutes(api chi.Router, pool *pgxpool.Pool, d *developerServices) {
	if d == nil {
		d = &developerServices{hooks: &webhooks.Handler{Pool: pool}}
	}
	api.Post("/api/webhooks", d.hooks.Create)
	api.Get("/api/webhooks", d.hooks.List)
	api.Patch("/api/webhooks/{id}", d.hooks.Update)
	api.Delete("/api/webhooks/{id}", d.hooks.Delete)
	api.Post("/api/webhooks/{id}/rotate", d.hooks.Rotate)
	api.Get("/api/webhooks/{id}/deliveries", d.hooks.Deliveries)
	txH := d.transaction
	if txH == nil {
		txH = &transactional.CompatibilityHandler{Pool: pool}
	}
	api.Post("/api/emails", txH.Send)
}
