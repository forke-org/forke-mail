package main

import (
	"context"
	"os"

	"forkemail/internal/webhooks"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type developerServices struct {
	hooks      *webhooks.Handler
	dispatcher *webhooks.Dispatcher
}

func newDeveloperServices(pool *pgxpool.Pool) (*developerServices, error) {
	h := &webhooks.Handler{Pool: pool}
	d := &developerServices{hooks: h}
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
}
