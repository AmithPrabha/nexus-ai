package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type source struct {
	Chunk   string `json:"chunk_id"`
	Title   string `json:"title"`
	Version string `json:"version"`
	Page    int    `json:"page"`
	Clause  string `json:"clause"`
	Excerpt string `json:"excerpt"`
}
type candidate struct {
	Answer    string `json:"answer"`
	Abstained bool   `json:"abstained"`
	Citations []struct {
		ID string `json:"chunk_id"`
	} `json:"citations"`
	Reason string `json:"reason"`
}
type policyAnswer struct {
	Answer    string   `json:"answer"`
	Abstained bool     `json:"abstained"`
	Citations []source `json:"citations"`
	Telemetry data     `json:"telemetry"`
}

func abstention() policyAnswer {
	return policyAnswer{Answer: "The assigned policies provide insufficient evidence.", Abstained: true, Citations: []source{}}
}
func (a *app) resolve(ctx context.Context, work string, c candidate) (policyAnswer, error) {
	out := abstention()
	if c.Abstained || len(c.Citations) != 1 {
		return out, nil
	}
	id, err := uuid.Parse(c.Citations[0].ID)
	if err != nil {
		return out, nil
	}
	var s source
	err = a.db.QueryRow(ctx, `SELECT c.id,p.title,p.version,c.page,c.clause,c.body FROM policy_chunks c JOIN policies p ON p.id=c.policy_id JOIN workspace_policies w ON w.policy_id=p.id WHERE c.id=$1 AND w.workspace_id=$2`, id, work).Scan(&s.Chunk, &s.Title, &s.Version, &s.Page, &s.Clause, &s.Excerpt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if c.Answer != s.Excerpt {
		return out, nil
	}
	return policyAnswer{Answer: s.Excerpt, Citations: []source{s}}, nil
}
func (a *app) ask(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Question string `json:"question"`
	}
	if err := decode(w, r, &req); err != nil {
		return err
	}
	q := strings.TrimSpace(req.Question)
	if q == "" || len([]rune(q)) > 1000 {
		return invalid
	}
	ctx := r.Context()
	work := scope(r)
	if err := a.limit(ctx, quotaKeys(a, r), 10, 30); err != nil {
		return err
	}
	started := time.Now()
	payload, _ := json.Marshal(data{"workspace_id": work, "question": q})
	deadline, done := context.WithTimeout(ctx, 8*time.Second)
	defer done()
	call, err := http.NewRequestWithContext(deadline, "POST", "http://ai-api:8000/retrieve-answer", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	call.Header.Set("Authorization", "Bearer "+a.secret)
	call.Header.Set("Content-Type", "application/json")
	call.Header.Set("X-Request-ID", w.Header().Get("X-Request-ID"))
	out := abstention()
	resp, err := http.DefaultClient.Do(call)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			err = unavailable
		} else {
			var c candidate
			if json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&c) == nil {
				out, err = a.resolve(ctx, work, c)
			}
		}
	}
	latency := time.Since(started).Milliseconds()
	result := "answered"
	if err != nil {
		result = "failed"
	} else if out.Abstained {
		result = "abstained"
	}
	_, dbErr := a.db.Exec(ctx, "INSERT INTO ai_runs(workspace_id,operation,mode,latency_ms,status) VALUES($1,'policy','mock',$2,$3)", work, latency, result)
	if dbErr != nil {
		return dbErr
	}
	if err != nil {
		return unavailable
	}
	out.Telemetry = data{"mode": "mock", "provider": "mock", "model": nil, "latency_ms": latency, "input_tokens": nil, "output_tokens": nil, "estimated_cost_aed": nil}
	return send(w, 200, out)
}
