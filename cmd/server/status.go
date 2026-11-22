// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"forkemail/internal/config"
	"forkemail/internal/statusmail"
	"forkemail/internal/transactional"

	"github.com/jackc/pgx/v5/pgxpool"
)

func statusMail(cfg config.Config, pool *pgxpool.Pool, dispatcher *transactional.Dispatcher) *statusmail.Handler {
	h := &statusmail.Handler{Pool: pool, WorkspaceID: cfg.AlertWorkspaceID, From: cfg.StatusFrom,
		BaseURL: cfg.PublicBaseURL, PageURL: cfg.StatusPageURL, Secret: []byte(cfg.UnsubscribeSecret)}
	if dispatcher != nil {
		h.Mail = transactional.Service{Pool: pool, Dispatcher: dispatcher}
	}
	return h
}
