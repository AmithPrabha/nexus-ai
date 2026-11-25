package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type invoice struct {
	ID       string    `json:"invoice_id"`
	Sample   string    `json:"sample_id"`
	Status   string    `json:"status"`
	Version  int       `json:"version"`
	Revision int       `json:"revision_no"`
	Created  time.Time `json:"created_at"`
	Job      string    `json:"job_id"`
}

const invoiceSelect = `SELECT i.id,i.sample_id,i.status,i.version,i.current_revision,i.created_at,
 (SELECT j.id FROM jobs j WHERE j.invoice_id=i.id ORDER BY j.created_at DESC,j.id LIMIT 1)
 FROM invoices i `

func readInvoice(row pgx.Row) (invoice, error) {
	var i invoice
	err := row.Scan(&i.ID, &i.Sample, &i.Status, &i.Version, &i.Revision, &i.Created, &i.Job)
	return i, noRows(err)
}
func noRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return missing
	}
	return err
}
func pathID(r *http.Request) (string, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return "", missing
	}
	return id.String(), nil
}
func transact(ctx context.Context, db *app, workspace string, op func(pgx.Tx) error) error {
	locked := func(tx pgx.Tx) error {
		// Serialize duplicate checks and transitions for this ownership scope.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,731))", workspace); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, "SELECT id FROM active_tenants WHERE id=$1", workspace).Scan(&id); err != nil {
			return noRows(err)
		}
		return op(tx)
	}
	if tx, ok := ctx.Value(transactionKey{}).(pgx.Tx); ok {
		return locked(tx)
	}
	return pgx.BeginFunc(ctx, db.db, locked)
}
func audit(ctx context.Context, tx pgx.Tx, id, actor, action string, from any, to, reason, requestID string) error {
	var userID, role *string
	if m := principal(ctx).Member; m != nil {
		userID, role = &m.UserID, &m.Role
		actor = m.DisplayName
	}
	_, err := tx.Exec(ctx, "INSERT INTO audit_events(invoice_id,actor,action,from_state,to_state,reason,request_id,actor_user_id,actor_role,ruleset) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'aed-demo-v1')", id, actor, action, from, to, reason, requestID, userID, role)
	return err
}
func quotaKeys(a *app, r *http.Request) []string {
	return []string{"ai:s:" + scope(r), "ai:ip:" + a.ipKey(r)}
}
func (a *app) enqueue(retry bool) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		ctx := r.Context()
		work := scope(r)
		key := r.Header.Get("Idempotency-Key")
		if len(key) < 1 || len(key) > 128 || strings.TrimSpace(key) != key {
			return invalid
		}
		var sample, id string
		var version int
		if retry {
			var req struct {
				Version int `json:"version"`
			}
			if err := decode(w, r, &req); err != nil || req.Version < 1 {
				return invalid
			}
			version = req.Version
			var err error
			id, err = pathID(r)
			if err != nil {
				return err
			}
		} else {
			var req struct {
				Sample string `json:"sample_id"`
			}
			if err := decode(w, r, &req); err != nil || !samplePattern.MatchString(req.Sample) {
				return invalid
			}
			sample = req.Sample
		}
		hash := sha256.Sum256([]byte(r.URL.Path + "|" + sample + "|" + strconv.Itoa(version)))
		digest := hex.EncodeToString(hash[:])
		var response json.RawMessage
		err := transact(ctx, a, work, func(tx pgx.Tx) error {
			var old string
			err := tx.QueryRow(ctx, "SELECT request_hash,response FROM idempotency_keys WHERE workspace_id=$1 AND key=$2", work, key).Scan(&old, &response)
			if err == nil {
				if old != digest {
					return conflict
				}
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			var from any
			if retry {
				i, err := readInvoice(tx.QueryRow(ctx, invoiceSelect+"WHERE i.id=$1 AND workspace_id=$2", id, work))
				if err != nil {
					return err
				}
				if i.Status != "FAILED" || i.Version != version {
					return conflict
				}
				from = i.Status
				if _, err := tx.Exec(ctx, "UPDATE invoices SET status='QUEUED',version=version+1 WHERE id=$1", id); err != nil {
					return err
				}
			} else {
				err = tx.QueryRow(ctx, "INSERT INTO invoices(workspace_id,sample_id,status) SELECT $1,sample_id,'QUEUED' FROM samples WHERE sample_id=$2 RETURNING id", work, sample).Scan(&id)
				if errors.Is(err, pgx.ErrNoRows) {
					return invalid
				}
				if err != nil {
					return err
				}
			}
			if err := a.limit(ctx, quotaKeys(a, r), 10, 30); err != nil {
				return err
			}
			var job string
			if err = tx.QueryRow(ctx, "INSERT INTO jobs(invoice_id) VALUES($1) RETURNING id", id).Scan(&job); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO outbox(job_id) VALUES($1)", job); err != nil {
				return err
			}
			if err = audit(ctx, tx, id, "demo_reviewer", "enqueue", from, "QUEUED", "Sample selected", w.Header().Get("X-Request-ID")); err != nil {
				return err
			}
			response, err = json.Marshal(data{"invoice_id": id, "job_id": job, "status": "QUEUED"})
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, "INSERT INTO idempotency_keys VALUES($1,$2,$3,$4)", work, key, digest, response)
			return err
		})
		if err != nil {
			return err
		}
		return send(w, 202, response)
	}
}
func (a *app) detail(ctx context.Context, tx pgx.Tx, work, id string) (data, error) {
	i, err := readInvoice(tx.QueryRow(ctx, invoiceSelect+"WHERE workspace_id=$1 AND i.id=$2", work, id))
	if err != nil {
		return nil, err
	}
	res := data{"invoice": i, "revision": nil, "validation_results": []rule{}, "version": i.Version}
	if i.Revision > 0 {
		var f, e, w, checks json.RawMessage
		if err := tx.QueryRow(ctx, "SELECT fields,evidence,warnings,validation_results FROM invoice_revisions WHERE invoice_id=$1 AND revision_no=$2", id, i.Revision).Scan(&f, &e, &w, &checks); err != nil {
			return nil, err
		}
		res["revision"] = data{"fields": f, "evidence": e, "warnings": w, "mode": "mock"}
		res["validation_results"] = checks
	}
	var notification *string
	if err := tx.QueryRow(ctx, "SELECT (SELECT delivery_state FROM sandbox_notifications WHERE invoice_id=$1 ORDER BY version DESC LIMIT 1)", id).Scan(&notification); err != nil {
		return nil, err
	}
	res["sandbox_notification"] = notification
	return res, nil
}
func (a *app) getInvoice(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	tx := r.Context().Value(transactionKey{}).(pgx.Tx)
	result, err := a.detail(r.Context(), tx, scope(r), id)
	if err != nil {
		return err
	}
	return send(w, 200, result)
}
func pageLimit(r *http.Request) (int, error) {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return 20, nil
	}
	i, err := strconv.Atoi(s)
	if err != nil || i < 1 || i > 100 {
		return 0, invalid
	}
	return i, nil
}
func (a *app) listInvoices(w http.ResponseWriter, r *http.Request) error {
	n, err := pageLimit(r)
	if err != nil {
		return err
	}
	cursor := r.URL.Query().Get("cursor")
	if cursor != "" {
		parsed, err := uuid.Parse(cursor)
		if err != nil {
			return invalid
		}
		cursor = parsed.String()
	}
	rows, err := a.reader(r.Context()).Query(r.Context(), invoiceSelect+`WHERE workspace_id=$1 AND ($2='' OR status=$2) AND (NULLIF($3,'')::uuid IS NULL OR i.id<NULLIF($3,'')::uuid) ORDER BY i.id DESC LIMIT $4`, scope(r), r.URL.Query().Get("status"), cursor, n+1)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []invoice{}
	for rows.Next() {
		i, err := readInvoice(rows)
		if err != nil {
			return err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var next any
	if len(items) > n {
		items = items[:n]
		next = items[n-1].ID
	}
	return send(w, 200, data{"items": items, "next_cursor": next})
}
func (a *app) jobInfo(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var status string
	var attempts int
	var code *string
	err = a.reader(r.Context()).QueryRow(r.Context(), "SELECT j.status,j.attempts,j.error_code FROM jobs j JOIN invoices i ON i.id=j.invoice_id WHERE j.id=$1 AND i.workspace_id=$2", id, scope(r)).Scan(&status, &attempts, &code)
	if err != nil {
		return noRows(err)
	}
	return send(w, 200, data{"job_id": id, "status": status, "attempts": attempts, "error_code": code})
}
func (a *app) history(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	n, err := pageLimit(r)
	if err != nil {
		return err
	}
	cursor := int64(0)
	if str := r.URL.Query().Get("cursor"); str != "" {
		cursor, err = strconv.ParseInt(str, 10, 64)
		if err != nil || cursor < 0 {
			return invalid
		}
	}
	var own bool
	if err = a.reader(ctx).QueryRow(ctx, "SELECT EXISTS(SELECT FROM invoices WHERE id=$1 AND workspace_id=$2)", id, scope(r)).Scan(&own); err != nil {
		return err
	}
	if !own {
		return missing
	}
	// The original demo schema rejects unknown keys. Only expose the extended
	// provenance to organization clients while retaining it in the database.
	hidden := []string{"invoice_id"}
	if principal(ctx).Member == nil {
		hidden = append(hidden, "tenant_id", "actor_user_id", "actor_role", "invoice_version", "revision_no", "ruleset")
	}
	rows, err := a.reader(ctx).Query(ctx, `SELECT e.id,to_jsonb(e)-$5::text[] FROM audit_events e JOIN invoices i ON i.id=e.invoice_id WHERE i.workspace_id=$1 AND i.id=$2 AND e.id>$3 ORDER BY e.id LIMIT $4`, scope(r), id, cursor, n+1, hidden)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []json.RawMessage{}
	var next any
	for rows.Next() {
		var eid int64
		var item json.RawMessage
		if err := rows.Scan(&eid, &item); err != nil {
			return err
		}
		if len(items) == n {
			next = strconv.FormatInt(cursor, 10)
			break
		}
		items = append(items, item)
		cursor = eid
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return send(w, 200, data{"items": items, "next_cursor": next})
}
func revision(ctx context.Context, tx pgx.Tx, i invoice, work, actor, reason, requestID string, x extraction) error {
	var dup bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM invoices o JOIN invoice_revisions rev ON rev.invoice_id=o.id AND rev.revision_no=o.current_revision WHERE o.workspace_id=$1 AND o.id<>$2 AND $3<>'' AND $4<>'' AND trim(rev.fields->>'supplier_trn')=$3 AND upper(trim(rev.fields->>'invoice_number'))=upper($4))`, work, i.ID, x.Fields.value("supplier_trn"), x.Fields.value("invoice_number")).Scan(&dup)
	if err != nil {
		return err
	}
	rules := validate(x, dup)
	status := "READY_FOR_APPROVAL"
	if len(rules) > 0 {
		status = "PENDING_REVIEW"
	}
	var actorID *string
	if m := principal(ctx).Member; m != nil {
		actorID = &m.UserID
		actor = m.DisplayName
	}
	_, err = tx.Exec(ctx, "INSERT INTO invoice_revisions(invoice_id,revision_no,fields,evidence,warnings,validation_results,actor,actor_user_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", i.ID, i.Revision+1, x.Fields, x.Evidence, x.Warnings, rules, actor, actorID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE invoices SET status=$2,version=version+1,current_revision=current_revision+1 WHERE id=$1", i.ID, status)
	if err != nil {
		return err
	}
	if status == "PENDING_REVIEW" {
		codes := []string{}
		for _, r := range rules {
			codes = append(codes, r.Code)
		}
		_, err = tx.Exec(ctx, "INSERT INTO sandbox_notifications(invoice_id,version,reason_codes,delivery_state) SELECT $1,$2,$3,'PENDING' WHERE EXISTS(SELECT FROM tenants WHERE id=$4 AND kind='DEMO')", i.ID, i.Version+1, codes, work)
		if err != nil {
			return err
		}
	}
	return audit(ctx, tx, i.ID, actor, "revision", i.Status, status, reason, requestID)
}
func reviewable(status string) bool {
	return status == "READY_FOR_APPROVAL" || status == "PENDING_REVIEW"
}
func (a *app) review(decision bool) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := pathID(r)
		if err != nil {
			return err
		}
		ctx := r.Context()
		work := scope(r)
		var version int
		var patch fields
		var action, reason string
		if decision {
			var req struct {
				Version  int    `json:"version"`
				Decision string `json:"decision"`
				Reason   string `json:"reason"`
			}
			if err := decode(w, r, &req); err != nil {
				return err
			}
			version, action, reason = req.Version, req.Decision, req.Reason
			if action != "approve" && action != "reject" {
				return invalid
			}
		} else {
			var req struct {
				Version int    `json:"version"`
				Fields  fields `json:"fields"`
				Reason  string `json:"reason"`
			}
			if err := decode(w, r, &req); err != nil {
				return err
			}
			version, patch, reason = req.Version, req.Fields, req.Reason
			if len(patch) == 0 || !validFields(patch) {
				return invalid
			}
		}
		reason = strings.TrimSpace(reason)
		if version < 1 || len(reason) > 1000 {
			return invalid
		}
		var result data
		err = transact(ctx, a, work, func(tx pgx.Tx) error {
			i, err := readInvoice(tx.QueryRow(ctx, invoiceSelect+"WHERE i.id=$1 AND workspace_id=$2", id, work))
			if err != nil {
				return err
			}
			if !reviewable(i.Status) || version != i.Version {
				return conflict
			}
			if m := principal(ctx).Member; m != nil && m.Role == "REVIEWER" && decision && action == "approve" && i.Status == "PENDING_REVIEW" {
				return denied
			}
			if reason == "" && (!decision || action == "reject" || i.Status == "PENDING_REVIEW") {
				return invalid
			}
			if decision {
				status := "APPROVED"
				if action == "reject" {
					status = "REJECTED"
				}
				if _, err = tx.Exec(ctx, "UPDATE invoices SET status=$2,version=version+1 WHERE id=$1", id, status); err != nil {
					return err
				}
				result = data{"invoice_id": id, "version": i.Version + 1, "status": status}
				return audit(ctx, tx, id, "demo_reviewer", action, i.Status, status, reason, w.Header().Get("X-Request-ID"))
			}
			var x extraction
			if err = tx.QueryRow(ctx, "SELECT fields,evidence,warnings FROM invoice_revisions WHERE invoice_id=$1 AND revision_no=$2", id, i.Revision).Scan(&x.Fields, &x.Evidence, &x.Warnings); err != nil {
				return err
			}
			for k, v := range patch {
				x.Fields[k] = v
				delete(x.Evidence, k)
			}
			if err = revision(ctx, tx, i, work, "demo_reviewer", reason, w.Header().Get("X-Request-ID"), x); err != nil {
				return err
			}
			result, err = a.detail(ctx, tx, work, id)
			return err
		})
		if err != nil {
			return err
		}
		return send(w, 200, result)
	}
}
func (a *app) invoiceRoutes(s chi.Router) {
	s.Get("/invoices", a.tenantEndpoint(readAccess, a.listInvoices))
	s.Post("/invoices", a.tenantEndpoint(writeInvoice, a.enqueue(false)))
	s.Get("/invoices/{id}", a.tenantEndpoint(readAccess, a.getInvoice))
	s.Get("/jobs/{id}", a.tenantEndpoint(readAccess, a.jobInfo))
	s.Patch("/invoices/{id}/review", a.tenantEndpoint(writeInvoice, a.review(false)))
	s.Post("/invoices/{id}/decision", a.tenantEndpoint(writeInvoice, a.review(true)))
	s.Post("/invoices/{id}/retry", a.tenantEndpoint(writeInvoice, a.enqueue(true)))
	s.Get("/invoices/{id}/audit", a.tenantEndpoint(readAccess, a.history))
}
