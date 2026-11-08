package webhooks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dispatcher struct {
	Pool   *pgxpool.Pool
	Vault  *Vault
	Client *http.Client
}
type job struct {
	id, workspace, endpoint, lease, url, payload, encrypted string
	version, attempts                                       int
	created                                                 time.Time
}

func (d *Dispatcher) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			t := time.NewTicker(250 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if _, err := d.Once(ctx); err != nil && ctx.Err() == nil {
						slog.Error("webhook dispatch failed", "error", "database_or_dispatch_error")
					}
				}
			}
		})
	}
	wg.Wait()
}

func (d *Dispatcher) claim(ctx context.Context) (job, error) {
	var j job
	err := pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE webhook_deliveries SET status=CASE WHEN attempts>=8 OR created_at<now()-interval '24 hours' THEN 'failed' ELSE 'pending' END,lease_token=NULL,claimed_at=NULL,last_error='lease_expired' WHERE status='processing' AND claimed_at<now()-interval '30 seconds'`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE webhook_deliveries SET status='failed',finished_at=now(),last_error='expired' WHERE status='pending' AND (attempts>=8 OR created_at<now()-interval '24 hours')`)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT d.id,d.workspace_id,d.endpoint_id,e.url,d.payload,e.secret,e.secret_version,d.attempts,d.created_at
		FROM webhook_deliveries d JOIN webhook_endpoints e ON e.id=d.endpoint_id AND e.workspace_id=d.workspace_id
		WHERE d.status='pending' AND d.next_attempt_at<=now() AND e.enabled AND e.next_dispatch_at<=now()
		ORDER BY d.next_attempt_at,d.id LIMIT 1 FOR UPDATE OF d,e SKIP LOCKED`).Scan(&j.id, &j.workspace, &j.endpoint, &j.url, &j.payload, &j.encrypted, &j.version, &j.attempts, &j.created)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `UPDATE webhook_deliveries SET status='processing',attempts=attempts+1,claimed_at=now(),lease_token=gen_random_uuid() WHERE id=$1 AND workspace_id=$2 RETURNING lease_token`, j.id, j.workspace).Scan(&j.lease)
		if err != nil {
			return err
		}
		j.attempts++
		_, err = tx.Exec(ctx, `UPDATE webhook_endpoints SET next_dispatch_at=now()+interval '1 second' WHERE id=$1 AND workspace_id=$2`, j.endpoint, j.workspace)
		return err
	})
	return j, err
}

func retryDelay(attempt int) time.Duration {
	return min(time.Hour, 5*time.Second*time.Duration(1<<min(attempt-1, 10))) + time.Duration(rand.IntN(1000))*time.Millisecond
}

func (d *Dispatcher) Once(ctx context.Context) (bool, error) {
	j, err := d.claim(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	status, reason, code := "failed", "invalid_signing_secret", 0
	secret, err := d.Vault.Open(j.encrypted, j.workspace)
	if err == nil {
		if _, err = parseURL(j.url); err == nil {
			requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			req, reqerr := http.NewRequestWithContext(requestCtx, "POST", j.url, bytes.NewBufferString(j.payload))
			if reqerr != nil {
				return true, reqerr
			}
			ts := strconv.FormatInt(time.Now().Unix(), 10)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Webhook-Id", j.id)
			req.Header.Set("Webhook-Timestamp", ts)
			req.Header.Set("Webhook-Key-Version", strconv.Itoa(j.version))
			req.Header.Set("Webhook-Signature", Signature(secret, j.id, ts, []byte(j.payload)))
			client := d.Client
			if client == nil {
				client = (Network{}).Client()
			}
			res, senderr := client.Do(req)
			reason = "network_error"
			status = "pending"
			if senderr == nil {
				code = res.StatusCode
				_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 8192))
				res.Body.Close()
				switch {
				case code >= 200 && code < 300:
					status = "delivered"
					reason = ""
				case code == 429 || code >= 500:
					reason = "receiver_temporary_error"
				default:
					status = "failed"
					reason = "receiver_rejected"
				}
			}
		} else {
			reason = "invalid_webhook_url"
		}
	}
	if status == "pending" && (j.attempts >= 8 || time.Since(j.created) >= 24*time.Hour) {
		status = "failed"
		reason = "retry_limit"
	}
	delay := retryDelay(j.attempts).Seconds()
	_, err = d.Pool.Exec(ctx, `UPDATE webhook_deliveries SET status=$4,response_code=NULLIF($5,0),last_error=NULLIF($6,''),next_attempt_at=now()+make_interval(secs=>$7),finished_at=CASE WHEN $4='pending' THEN NULL ELSE now() END,lease_token=NULL,claimed_at=NULL WHERE id=$1 AND workspace_id=$2 AND lease_token=$3 AND status='processing'`, j.id, j.workspace, j.lease, status, code, reason, delay)
	return true, err
}
