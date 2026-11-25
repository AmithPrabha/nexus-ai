package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
)

func (a *app) metrics(w http.ResponseWriter, r *http.Request) error {
	var invoices, pending, runs, failures, notes int
	var latency *float64
	err := a.db.QueryRow(r.Context(), `SELECT
      (SELECT count(*) FROM invoices WHERE workspace_id=$1),
      (SELECT count(*) FROM invoices WHERE workspace_id=$1 AND status IN ('READY_FOR_APPROVAL','PENDING_REVIEW')),
      count(*),count(*) FILTER(WHERE status='failed'),avg(latency_ms)::float8,
      (SELECT count(*) FROM sandbox_notifications n JOIN invoices i ON i.id=n.invoice_id WHERE i.workspace_id=$1 AND n.delivery_state='RECORDED')
      FROM ai_runs WHERE workspace_id=$1`, scope(r)).Scan(&invoices, &pending, &runs, &failures, &latency, &notes)
	if err != nil {
		return err
	}
	return send(w, 200, data{"mode": "mock", "invoices": invoices, "awaiting_review": pending, "runs": runs, "failed_runs": failures, "average_operation_ms": latency, "sandbox_notifications": notes, "input_tokens": nil, "output_tokens": nil, "estimated_cost_aed": nil})
}
func evaluation(w http.ResponseWriter, r *http.Request) error {
	b, err := os.ReadFile("/evaluation/report.json")
	if errors.Is(err, os.ErrNotExist) {
		return send(w, 200, data{"status": "not_run"})
	}
	if err != nil || !json.Valid(b) {
		return unavailable
	}
	return send(w, 200, json.RawMessage(b))
}
func (a *app) reset(w http.ResponseWriter, r *http.Request) error {
	if r.ContentLength != 0 {
		var empty struct{}
		if err := decode(w, r, &empty); err != nil {
			return err
		}
	}
	_, err := a.db.Exec(r.Context(), "UPDATE workspaces SET expires_at=now() WHERE id=$1", scope(r))
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "nexus_session", Path: "/api/v1", Value: "", MaxAge: -1, Secure: strings.HasPrefix(a.origin, "https://"), HttpOnly: true, SameSite: http.SameSiteLaxMode})
	return send(w, 200, data{"status": "expired"})
}
