package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type member struct {
	TenantID    string `json:"tenant_id"`
	TenantName  string `json:"tenant_name"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type database interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type transactionKey struct{}

func (a *app) reader(ctx context.Context) database {
	if tx, ok := ctx.Value(transactionKey{}).(pgx.Tx); ok {
		return tx
	}
	return a.db
}

func digest(token string) []byte { hash := sha256.Sum256([]byte(token)); return hash[:] }

func (a *app) memberSession(ctx context.Context, db database, token string) (session, error) {
	var s session
	if a.organizationsDisabled {
		return s, unauth
	}
	var m member
	err := db.QueryRow(ctx, `SELECT tenant_id,tenant_name,user_id,display_name,role,expires_at FROM member_context($1)`, digest(token)).Scan(&m.TenantID, &m.TenantName, &m.UserID, &m.DisplayName, &m.Role, &s.Expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, unauth
	}
	s.ID = m.TenantID
	s.Member = &m
	s.CSRF = a.csrf(token)
	return s, err
}

func (a *app) sessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	c := &http.Cookie{Name: "nexus_session", Path: "/api/v1", Value: token, Expires: expires, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: strings.HasPrefix(a.origin, "https://")}
	if token == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

func (a *app) login(w http.ResponseWriter, r *http.Request) error {
	if a.organizationsDisabled {
		return forbidden
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(w, r, &req); err != nil {
		return err
	}
	if err := a.limit(r.Context(), []string{"login:" + a.ipKey(r)}, 60); err != nil {
		return err
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(req.Code)
	if err != nil || len(raw) != 32 {
		return unauth
	}
	token := randomToken()
	var s session
	err = pgx.BeginFunc(r.Context(), a.db, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(r.Context(), "SELECT redeem_login($1,$2)", digest(req.Code), digest(token)).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return unauth
		}
		if old, err := r.Cookie("nexus_session"); err == nil {
			if _, err = tx.Exec(r.Context(), "DELETE FROM user_sessions WHERE digest=$1", digest(old.Value)); err != nil {
				return err
			}
		}
		var err error
		s, err = a.memberSession(r.Context(), tx, token)
		return err
	})
	if err != nil {
		return err
	}
	a.sessionCookie(w, token, s.Expires)
	return send(w, http.StatusCreated, s.organizationContext())
}

func (s session) organizationContext() data {
	return data{"member": s.Member, "expires_at": s.Expires, "csrf_token": s.CSRF}
}

func (a *app) identity(w http.ResponseWriter, r *http.Request) error {
	s := principal(r.Context())
	if s.Member == nil {
		return unauth
	}
	return send(w, http.StatusOK, s.organizationContext())
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) error {
	if principal(r.Context()).Member == nil {
		return forbidden
	}
	c, err := r.Cookie("nexus_session")
	if err != nil {
		return unauth
	}
	if _, err = a.db.Exec(r.Context(), "DELETE FROM user_sessions WHERE digest=$1", digest(c.Value)); err != nil {
		return err
	}
	a.sessionCookie(w, "", time.Time{})
	return send(w, http.StatusOK, data{"status": "signed_out"})
}

func principal(ctx context.Context) session { s, _ := ctx.Value(contextKey{}).(session); return s }

var denied = apiError{403, "PERMISSION_DENIED", "Your organization role does not permit this action."}

type permission int

const (
	readAccess permission = iota
	writeInvoice
)

// Authenticate again under a database lock, scope all domain statements on the
// same connection, and only release the JSON response after a successful commit.
func (a *app) tenantEndpoint(access permission, fn handler) http.HandlerFunc {
	return endpoint(func(w http.ResponseWriter, r *http.Request) error {
		ctx := r.Context()
		s := principal(ctx)
		out := &pendingJSON{ResponseWriter: w}
		opts := pgx.TxOptions{}
		if r.Method == http.MethodGet {
			opts.IsoLevel = pgx.RepeatableRead
		}
		err := pgx.BeginTxFunc(ctx, a.db, opts, func(tx pgx.Tx) error {
			if s.Member != nil {
				c, err := r.Cookie("nexus_session")
				if err != nil {
					return unauth
				}
				s, err = a.memberSession(ctx, tx, c.Value)
				if err != nil {
					return err
				}
				if access == writeInvoice && s.Member.Role == "VIEWER" {
					return denied
				}
			} else {
				var ok bool
				if err := tx.QueryRow(ctx, "SELECT true FROM workspaces WHERE id=$1 AND expires_at>now() FOR SHARE", s.ID).Scan(&ok); err != nil {
					return noRows(err)
				}
			}
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE nexus_request"); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "SELECT set_config('nexus.tenant',$1,true)", s.ID); err != nil {
				return err
			}
			scoped := context.WithValue(context.WithValue(ctx, contextKey{}, s), transactionKey{}, tx)
			return fn(out, r.WithContext(scoped))
		})
		if err != nil {
			return err
		}
		return send(w, out.status, out.body)
	})
}
