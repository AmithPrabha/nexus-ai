package main

import (
	"context"
	"crypto/hmac"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

func (a *app) dispatch(ctx context.Context) {
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			err := pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
				var event, job, work, doc string
				err := tx.QueryRow(ctx, `SELECT o.id,j.id,i.workspace_id,i.sample_id FROM outbox o JOIN jobs j ON j.id=o.job_id JOIN invoices i ON i.id=j.invoice_id JOIN workspaces w ON w.id=i.workspace_id WHERE available_at<=now() AND w.expires_at>now() AND ((j.status='QUEUED' AND j.next_attempt_at<=now()) OR (j.status='PROCESSING' AND j.lease_until<=now())) ORDER BY available_at FOR UPDATE OF o SKIP LOCKED LIMIT 1`).Scan(&event, &job, &work, &doc)
				if errors.Is(err, pgx.ErrNoRows) {
					return nil
				}
				if err != nil {
					return err
				}
				publish, done := context.WithTimeout(ctx, 2*time.Second)
				defer done()
				if err = a.redis.XAdd(publish, &redis.XAddArgs{Stream: "invoice.jobs", MaxLen: 10000, Approx: true, Values: map[string]any{"event_id": event, "job_id": job, "workspace_id": work, "document_id": doc, "schema_version": 1}}).Err(); err != nil {
					return err
				}
				// Keep the durable intent until completion so stream loss or an expired lease can be repaired.
				_, err = tx.Exec(ctx, "UPDATE outbox SET available_at=now()+interval '10 seconds' WHERE id=$1", event)
				return err
			})
			if err != nil && ctx.Err() == nil {
				slog.Error("outbox dispatch deferred")
			}
		}
	}
}

type jobState struct {
	ID, Workspace, Status string
	Attempt               int
	Lease                 *string
	Expired, Due          bool
	Invoice               invoice
}

func (a *app) lockedJob(r *http.Request, fn func(pgx.Tx, jobState) error) error {
	ctx := r.Context()
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var work string
	err = a.db.QueryRow(ctx, "SELECT i.workspace_id FROM jobs j JOIN invoices i ON i.id=j.invoice_id WHERE j.id=$1", id).Scan(&work)
	if err != nil {
		return noRows(err)
	}
	return transact(ctx, a, work, func(tx pgx.Tx) error {
		j := jobState{ID: id, Workspace: work}
		var invoiceID string
		err := tx.QueryRow(ctx, "SELECT invoice_id,status,attempts,lease_token,COALESCE(lease_until<=now(),true),next_attempt_at<=now() FROM jobs WHERE id=$1", id).Scan(&invoiceID, &j.Status, &j.Attempt, &j.Lease, &j.Expired, &j.Due)
		if err != nil {
			return noRows(err)
		}
		j.Invoice, err = readInvoice(tx.QueryRow(ctx, invoiceSelect+"WHERE i.id=$1 AND workspace_id=$2", invoiceID, work))
		if err != nil {
			return err
		}
		return fn(tx, j)
	})
}
func failJob(ctx context.Context, tx pgx.Tx, j jobState, code, reqID string, retry bool) error {
	status := "FAILED"
	if retry && j.Attempt < 3 {
		status = "QUEUED"
	}
	_, err := tx.Exec(ctx, `UPDATE jobs SET status=$2,error_code=$3,next_attempt_at=now()+((power(2,attempts)+random())*interval '1 second') WHERE id=$1`, j.ID, status, code)
	if err != nil {
		return err
	}
	if status == "FAILED" {
		_, err = tx.Exec(ctx, "DELETE FROM outbox WHERE job_id=$1", j.ID)
	} else {
		_, err = tx.Exec(ctx, "UPDATE outbox SET available_at=(SELECT next_attempt_at FROM jobs WHERE id=$1) WHERE job_id=$1", j.ID)
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE invoices SET status=$2,version=version+1 WHERE id=$1", j.Invoice.ID, status); err != nil {
		return err
	}
	return audit(ctx, tx, j.Invoice.ID, "worker", "failure", j.Invoice.Status, status, code, reqID)
}
func (a *app) claim(w http.ResponseWriter, r *http.Request) error {
	var empty struct{}
	if err := decode(w, r, &empty); err != nil {
		return err
	}
	ctx := r.Context()
	var res data
	err := a.lockedJob(r, func(tx pgx.Tx, j jobState) error {
		res = data{"disposition": "terminal"}
		if j.Status == "FAILED" || j.Status == "SUCCEEDED" {
			return nil
		}
		if j.Status == "PROCESSING" && !j.Expired || !j.Due {
			res = data{"disposition": "busy"}
			return nil
		}
		if j.Attempt >= 3 {
			return failJob(ctx, tx, j, "LEASE_EXHAUSTED", w.Header().Get("X-Request-ID"), false)
		}
		token := randomToken()
		var until time.Time
		err := tx.QueryRow(ctx, "UPDATE jobs SET status='PROCESSING',attempts=attempts+1,lease_token=$2,lease_until=now()+interval '120 seconds',error_code=NULL WHERE id=$1 RETURNING lease_until", j.ID, token).Scan(&until)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE invoices SET status='PROCESSING',version=version+1 WHERE id=$1", j.Invoice.ID); err != nil {
			return err
		}
		if err = audit(ctx, tx, j.Invoice.ID, "worker", "claim", j.Invoice.Status, "PROCESSING", "Extraction attempt", w.Header().Get("X-Request-ID")); err != nil {
			return err
		}
		res = data{"disposition": "claimed", "sample_id": j.Invoice.Sample, "attempt": j.Attempt + 1, "lease_token": token, "expires_at": until}
		return nil
	})
	if err != nil {
		return err
	}
	return send(w, 200, res)
}

type attempt struct {
	Attempt int    `json:"attempt"`
	Lease   string `json:"lease_token"`
	Latency int    `json:"latency_ms"`
}

func (a *app) finish(success bool) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		var att attempt
		var x extraction
		var code string
		var retry bool
		if success {
			var req struct {
				attempt
				Extraction extraction `json:"extraction"`
			}
			if err := decode(w, r, &req); err != nil {
				return err
			}
			att, x = req.attempt, req.Extraction
			if len(x.Fields) != len(fieldNames) || !validFields(x.Fields) || x.Evidence == nil || x.Warnings == nil {
				return invalid
			}
			for k, e := range x.Evidence {
				if _, ok := x.Fields[k]; !ok || e.Page < 1 || e.Page > 10 || len(e.Excerpt) > 512 || e.Excerpt == "" {
					return invalid
				}
			}
		} else {
			var req struct {
				attempt
				Code  string `json:"error_code"`
				Retry bool   `json:"retryable"`
			}
			if err := decode(w, r, &req); err != nil {
				return err
			}
			att, code, retry = req.attempt, req.Code, req.Retry
			switch code {
			case "OCR_REQUIRED", "INVALID_DOCUMENT", "PROVIDER_ERROR", "SCHEMA_ERROR":
			default:
				return invalid
			}
		}
		if att.Attempt < 1 || att.Latency < 0 || att.Latency > 120000 {
			return invalid
		}
		ctx := r.Context()
		res := data{"disposition": "accepted"}
		err := a.lockedJob(r, func(tx pgx.Tx, j jobState) error {
			if j.Attempt != att.Attempt || j.Lease == nil || !hmac.Equal([]byte(*j.Lease), []byte(att.Lease)) {
				return conflict
			}
			if success && j.Status == "SUCCEEDED" || !success && (j.Status == "FAILED" || j.Status == "QUEUED") {
				return nil
			}
			if j.Status != "PROCESSING" || j.Expired {
				return conflict
			}
			status := "failed"
			if success {
				status = "succeeded"
			}
			if _, err := tx.Exec(ctx, "INSERT INTO ai_runs(workspace_id,job_id,attempt,operation,mode,latency_ms,status) VALUES($1,$2,$3,'extract','mock',$4,$5)", j.Workspace, j.ID, j.Attempt, att.Latency, status); err != nil {
				return err
			}
			if !success {
				return failJob(ctx, tx, j, code, w.Header().Get("X-Request-ID"), retry)
			}
			if err := revision(ctx, tx, j.Invoice, j.Workspace, "worker", "Mock extraction completed", w.Header().Get("X-Request-ID"), x); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "UPDATE jobs SET status='SUCCEEDED' WHERE id=$1", j.ID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "DELETE FROM outbox WHERE job_id=$1", j.ID)
			return err
		})
		if err != nil {
			return err
		}
		return send(w, 200, res)
	}
}
func (a *app) internal() http.Handler {
	router := baseRouter()
	router.Group(func(private chi.Router) {
		private.Use(func(next http.Handler) http.Handler {
			return endpoint(func(w http.ResponseWriter, r *http.Request) error {
				if !hmac.Equal([]byte(r.Header.Get("Authorization")), []byte("Bearer "+a.secret)) {
					return forbidden
				}
				next.ServeHTTP(w, r)
				return nil
			})
		})
		private.Route("/internal/jobs/{id}", func(s chi.Router) {
			s.Post("/claim", endpoint(a.claim))
			s.Post("/complete", endpoint(a.finish(true)))
			s.Post("/fail", endpoint(a.finish(false)))
		})
	})
	router.Post("/internal/notifications/receive", endpoint(a.receiveNotification))
	return router
}
