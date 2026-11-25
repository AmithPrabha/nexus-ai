package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type app struct {
	db                    *pgxpool.Pool
	redis                 *redis.Client
	origin, secret        string
	workflow              bool
	organizationsDisabled bool
}
type apiError struct {
	status        int
	code, message string
}

func (e apiError) Error() string { return e.code }

var (
	invalid     = apiError{400, "VALIDATION_ERROR", "Check the request fields and try again."}
	missing     = apiError{404, "NOT_FOUND", "Resource not found."}
	forbidden   = apiError{403, "FORBIDDEN", "Request verification failed."}
	unauth      = apiError{401, "SESSION_REQUIRED", "Sign in or start a demo session."}
	conflict    = apiError{409, "CONFLICT", "Refresh the current state before retrying."}
	quota       = apiError{429, "QUOTA_EXCEEDED", "Demo allowance reached. Try again later."}
	unavailable = apiError{503, "UNAVAILABLE", "A service is temporarily unavailable."}
)

type data = map[string]any
type handler func(http.ResponseWriter, *http.Request) error

// Tenant handlers publish their JSON response after their transaction commits.
type pendingJSON struct {
	http.ResponseWriter
	status int
	body   any
}

func send(w http.ResponseWriter, status int, body any) error {
	if out, ok := w.(*pendingJSON); ok {
		out.status, out.body = status, body
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(body)
}
func endpoint(fn handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			var e apiError
			if !errors.As(err, &e) {
				e = unavailable
				slog.Error("dependency failure", "request_id", w.Header().Get("X-Request-ID"), "type", fmt.Sprintf("%T", err))
			}
			_ = send(w, e.status, data{"error": data{"code": e.code, "message": e.message, "request_id": w.Header().Get("X-Request-ID")}})
		}
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		return invalid
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return invalid
	}
	if d.Decode(new(any)) != io.EOF {
		return invalid
	}
	return nil
}
func randomToken() string {
	var buf [32]byte
	rand.Read(buf[:])
	return base64.RawURLEncoding.EncodeToString(buf[:])
}
func (a *app) csrf(token string) string {
	m := hmac.New(sha256.New, []byte(a.secret))
	m.Write([]byte("csrf:" + token))
	return hex.EncodeToString(m.Sum(nil))
}

type session struct {
	ID      string    `json:"session_id"`
	Expires time.Time `json:"expires_at"`
	CSRF    string    `json:"csrf_token"`
	Member  *member   `json:"member,omitempty"`
}

func (a *app) current(r *http.Request) (session, error) {
	var s session
	c, err := r.Cookie("nexus_session")
	if err != nil || len(c.Value) != 43 {
		return s, unauth
	}
	hash := sha256.Sum256([]byte(c.Value))
	err = a.db.QueryRow(r.Context(), "SELECT id,expires_at FROM workspaces WHERE session_hash=$1 AND expires_at>now()", hash[:]).Scan(&s.ID, &s.Expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return a.memberSession(r.Context(), a.db, c.Value)
	}
	s.CSRF = a.csrf(c.Value)
	return s, err
}

type contextKey struct{}

func scope(r *http.Request) string { return principal(r.Context()).ID }
func (a *app) scoped(next http.Handler) http.Handler {
	return endpoint(func(w http.ResponseWriter, r *http.Request) error {
		s, err := a.current(r)
		if err != nil {
			return err
		}
		if r.Method != "GET" && !hmac.Equal([]byte(s.CSRF), []byte(r.Header.Get("X-CSRF-Token"))) {
			return forbidden
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, s)))
		return nil
	})
}

var limitScript = redis.NewScript(`
for i,key in ipairs(KEYS) do
  if tonumber(redis.call('GET',key) or '0') >= tonumber(ARGV[i]) then return 0 end
end
for i,key in ipairs(KEYS) do
  if redis.call('INCR',key)==1 then redis.call('EXPIRE',key,3600) end
end
return 1`)

func (a *app) limit(ctx context.Context, keys []string, limits ...any) error {
	n, err := limitScript.Run(ctx, a.redis, keys, limits...).Int()
	if err != nil {
		return err
	}
	if n == 0 {
		return quota
	}
	return nil
}
func (a *app) ipKey(r *http.Request) string {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	m := hmac.New(sha256.New, []byte(a.secret))
	m.Write([]byte("ip:" + ip))
	return hex.EncodeToString(m.Sum(nil))
}
func (a *app) start(w http.ResponseWriter, r *http.Request) error {
	if r.ContentLength != 0 {
		var empty struct{}
		if err := decode(w, r, &empty); err != nil {
			return err
		}
	}
	if s, err := a.current(r); err == nil {
		if s.Member != nil {
			return forbidden
		}
		return send(w, 201, s)
	} else if !errors.Is(err, unauth) {
		return err
	}
	if err := a.limit(r.Context(), []string{"sessions:" + a.ipKey(r)}, 30); err != nil {
		return err
	}
	token := randomToken()
	hash := sha256.Sum256([]byte(token))
	s := session{CSRF: a.csrf(token)}
	if err := a.db.QueryRow(r.Context(), "INSERT INTO workspaces(session_hash) VALUES($1) RETURNING id,expires_at", hash[:]).Scan(&s.ID, &s.Expires); err != nil {
		return err
	}
	a.sessionCookie(w, token, s.Expires)
	return send(w, 201, s)
}
func (a *app) samples(w http.ResponseWriter, r *http.Request) error {
	rows, err := a.db.Query(r.Context(), "SELECT sample_id,title,description FROM samples ORDER BY sample_id")
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []data{}
	for rows.Next() {
		var id, title, description string
		if err := rows.Scan(&id, &title, &description); err != nil {
			return err
		}
		items = append(items, data{"sample_id": id, "title": title, "description": description, "preview_url": "/api/v1/samples/" + id + "/preview"})
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return send(w, 200, data{"items": items})
}
func (a *app) ready(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	var installed bool
	err := a.db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM schema_migrations WHERE version='008_identity_constraints.sql')").Scan(&installed)
	if err != nil || !installed || a.redis.Ping(ctx).Err() != nil {
		return unavailable
	}
	return send(w, 200, data{"status": "ready"})
}
func baseRouter() *chi.Mux {
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, done := context.WithTimeout(r.Context(), 12*time.Second)
			defer done()
			r = r.WithContext(ctx)
			w.Header().Set("X-Request-ID", uuid.NewString())
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			next.ServeHTTP(w, r)
		})
	})
	router.NotFound(endpoint(func(w http.ResponseWriter, r *http.Request) error { return missing }))
	router.MethodNotAllowed(endpoint(func(w http.ResponseWriter, r *http.Request) error {
		return apiError{405, "METHOD_NOT_ALLOWED", "Unsupported method."}
	}))
	return router
}
func (a *app) public() http.Handler {
	router := baseRouter()
	router.Get("/healthz", endpoint(func(w http.ResponseWriter, r *http.Request) error { return send(w, 200, data{"status": "healthy"}) }))
	router.Get("/readyz", endpoint(a.ready))
	router.Route("/api/v1", func(v chi.Router) {
		v.Use(func(next http.Handler) http.Handler {
			return endpoint(func(w http.ResponseWriter, r *http.Request) error {
				if r.Method != "GET" && r.Header.Get("Origin") != a.origin {
					return forbidden
				}
				next.ServeHTTP(w, r)
				return nil
			})
		})
		v.Post("/demo/sessions", endpoint(a.start))
		v.Post("/auth/login", endpoint(a.login))
		v.Group(func(s chi.Router) {
			s.Use(a.scoped)
			s.Get("/auth/session", endpoint(a.identity))
			s.Post("/auth/logout", endpoint(a.logout))
			a.invoiceRoutes(s)
			s.Get("/metrics/demo", a.tenantEndpoint(readAccess, a.metrics))
			s.Get("/evaluation", endpoint(evaluation))
			s.Post("/demo/reset", endpoint(a.reset))
			s.Post("/policy/questions", a.tenantEndpoint(readAccess, a.ask))
			s.Get("/samples", endpoint(a.samples))
			s.Get("/samples/{sample}/preview", endpoint(a.preview))
		})
	})
	return router
}
func server(addr string, router http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 75 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
}
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	origin := os.Getenv("PUBLIC_ORIGIN")
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" || origin != parsed.Scheme+"://"+parsed.Host || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1"))) {
		return errors.New("invalid PUBLIC_ORIGIN")
	}
	if len(os.Getenv("SERVICE_TOKEN")) < 32 || os.Getenv("DATABASE_URL") == "" || os.Getenv("REDIS_ADDR") == "" {
		return errors.New("missing service configuration")
	}
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer pool.Close()
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR")})
	defer client.Close()
	a := app{db: pool, redis: client, origin: origin, secret: os.Getenv("SERVICE_TOKEN")}
	pub := server(":8080", a.public())
	private := server(":8081", a.internal())
	a.organizationsDisabled = os.Getenv("ORGANIZATION_ACCESS") == "disabled"
	a.workflow = os.Getenv("NOTIFICATION_MODE") == "n8n"
	go a.notifications(ctx)
	go a.dispatch(ctx)
	errs := make(chan error, 2)
	go func() { errs <- private.ListenAndServe() }()
	go func() { errs <- pub.ListenAndServe() }()
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				_, err := pool.Exec(ctx, "DELETE FROM workspaces WHERE expires_at<=now(); DELETE FROM login_codes WHERE expires_at<=now(); DELETE FROM user_sessions WHERE expires_at<=now()")
				if err != nil && ctx.Err() == nil {
					slog.Error("session purge failed")
				}
			}
		}
	}()
	select {
	case <-ctx.Done():
	case err = <-errs:
		return err
	}
	shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	return errors.Join(pub.Shutdown(shutdown), private.Shutdown(shutdown))
}
func main() {
	if err := run(); err != nil {
		slog.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}
