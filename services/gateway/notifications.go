package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type reviewEvent struct {
	Schema    int      `json:"schema_version"`
	ID        string   `json:"event_id"`
	Timestamp int64    `json:"timestamp"`
	Invoice   string   `json:"invoice_id"`
	State     string   `json:"state"`
	Codes     []string `json:"reason_codes"`
	Demo      bool     `json:"demo"`
}
type signedEvent struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func (a *app) eventMAC(payload string) []byte {
	mac := hmac.New(sha256.New, []byte(a.secret))
	mac.Write([]byte("notification.v1." + payload))
	return mac.Sum(nil)
}
func (a *app) signEvent(e reviewEvent) (signedEvent, error) {
	encoded, err := json.Marshal(e)
	if err != nil {
		return signedEvent{}, err
	}
	payload := base64.RawURLEncoding.EncodeToString(encoded)
	return signedEvent{payload, hex.EncodeToString(a.eventMAC(payload))}, nil
}
func (a *app) verifyEvent(s signedEvent, now time.Time) (reviewEvent, error) {
	var e reviewEvent
	sig, err := hex.DecodeString(s.Signature)
	if err != nil || !hmac.Equal(sig, a.eventMAC(s.Payload)) {
		return e, forbidden
	}
	payload, err := base64.RawURLEncoding.DecodeString(s.Payload)
	if err != nil || json.Unmarshal(payload, &e) != nil || e.Schema != 1 || !e.Demo || e.State != "PENDING_REVIEW" || e.Timestamp < now.Unix()-300 || e.Timestamp > now.Unix()+300 {
		return e, forbidden
	}
	return e, nil
}
func (a *app) recordReceipt(ctx context.Context, s signedEvent) (string, error) {
	e, err := a.verifyEvent(s, time.Now())
	if err != nil {
		return "", err
	}
	var id string
	err = a.db.QueryRow(ctx, `SELECT n.id FROM sandbox_notifications n JOIN invoices i ON n.invoice_id=i.id JOIN workspaces w ON w.id=i.workspace_id WHERE n.id=$1 AND n.invoice_id=$2 AND w.expires_at>now()`, e.ID, e.Invoice).Scan(&id)
	if err != nil {
		return "", noRows(err)
	}
	_, err = a.db.Exec(ctx, "INSERT INTO sandbox_receipts(event_id) VALUES($1) ON CONFLICT DO NOTHING", id)
	return id, err
}
func (a *app) receiveNotification(w http.ResponseWriter, r *http.Request) error {
	var event signedEvent
	if err := decode(w, r, &event); err != nil {
		return err
	}
	id, err := a.recordReceipt(r.Context(), event)
	if err != nil {
		return err
	}
	return send(w, 200, data{"event_id": id})
}
func (a *app) deliver(ctx context.Context, e reviewEvent) error {
	msg, err := a.signEvent(e)
	if err != nil {
		return err
	}
	if !a.workflow {
		_, err = a.recordReceipt(ctx, msg)
		return err
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://n8n:5678/webhook/nexus-review", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var receipt struct {
		ID string `json:"event_id"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&receipt) != nil || receipt.ID != e.ID {
		return unavailable
	}
	return nil
}
func (a *app) notifications(ctx context.Context) {
	tick := time.NewTicker(700 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			err := pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
				var e reviewEvent
				var attempts int
				// NO KEY UPDATE allows the independent receiver to insert its foreign-key receipt.
				err := tx.QueryRow(ctx, `SELECT n.id,n.invoice_id,n.reason_codes,n.attempts FROM sandbox_notifications n JOIN invoices i ON i.id=n.invoice_id JOIN workspaces w ON w.id=i.workspace_id WHERE n.delivery_state='PENDING' AND n.available_at<=now() AND w.expires_at>now() ORDER BY n.available_at FOR NO KEY UPDATE OF n SKIP LOCKED LIMIT 1`).Scan(&e.ID, &e.Invoice, &e.Codes, &attempts)
				if errors.Is(err, pgx.ErrNoRows) {
					return nil
				}
				if err != nil {
					return err
				}
				e.Schema, e.Timestamp, e.State, e.Demo = 1, time.Now().Unix(), "PENDING_REVIEW", true
				delivery, done := context.WithTimeout(ctx, 5*time.Second)
				defer done()
				status := "RECORDED"
				if err = a.deliver(delivery, e); err != nil {
					status = "PENDING"
					if attempts+1 >= 3 {
						status = "FAILED"
					}
				}
				_, err = tx.Exec(ctx, `UPDATE sandbox_notifications SET delivery_state=$2,attempts=attempts+1,available_at=now()+(power(2,attempts+1)*interval '1 second') WHERE id=$1`, e.ID, status)
				return err
			})
			if err != nil && ctx.Err() == nil {
				slog.Error("sandbox notification deferred")
			}
		}
	}
}
