"use client";
import { useCallback, useEffect, useState } from "react";
import { api, headers, Model, statusText, value } from "../lib/client";

type Props = {
  id?: string;
  member: Model<"Member"> | null;
  samples: Model<"Sample">[];
  token: string;
  refresh: () => void;
};
const fieldLabel = (s: string) =>
  s.replaceAll("_", " ").replace(/^./, (x) => x.toUpperCase());
export function State({ status }: { status: string }) {
  return (
    <span className={"state s-" + status.toLowerCase()}>
      {statusText(status)}
    </span>
  );
}
export default function Auditor({ id, samples, token, member, refresh }: Props) {
  const [sample, choose] = useState("office-supplies");
  const [detail, setDetail] = useState<Model<"Detail"> | null>(null);
  const [audit, setAudit] = useState<Model<"Audit">[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  const [edits, setEdits] = useState<Partial<Model<"Fields">>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [key, setKey] = useState("");
  const [generation, reload] = useState(0);
  const load = useCallback(() => reload((n) => n + 1), []);
  useEffect(() => {
    if (!id) return;
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    let delay = 2000;
    async function poll() {
      try {
        const next = value(
          await api.GET("/api/v1/invoices/{id}", {
            params: { path: { id: id! } },
          }),
        );
        const timeline = value(
          await api.GET("/api/v1/invoices/{id}/audit", {
            params: { path: { id: id! }, query: { limit: 100 } },
          }),
        );
        if (!alive) return;
        setDetail(next);
        setAudit(timeline.items);
        setCursor(timeline.next_cursor);
        if (["QUEUED", "PROCESSING"].includes(next.invoice.status)) {
          await api.GET("/api/v1/jobs/{id}", {
            params: { path: { id: next.invoice.job_id } },
          });
          if (alive) {
            timer = setTimeout(poll, delay);
            delay = Math.min(delay * 1.5, 8000);
          }
        } else refresh();
      } catch (err) {
        if (alive) setError((err as Error).message);
      }
    }
    void poll();
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [id, generation, refresh]);
  const chosen = samples.find(
    (s) => s.sample_id === (detail?.invoice.sample_id || sample),
  );
  const status = detail?.invoice.status || "";
  const canWrite = member?.role !== "VIEWER";
  const canApprove = member?.role !== "REVIEWER" || status !== "PENDING_REVIEW";
  const editable = canWrite && ["READY_FOR_APPROVAL", "PENDING_REVIEW"].includes(status);
  async function action(fn: () => Promise<void>) {
    setError("");
    setBusy(true);
    try {
      await fn();
    } catch (err) {
      setError((err as Error).message);
      if (id) load();
    } finally {
      setBusy(false);
    }
  }
  function submit() {
    void action(async () => {
      const requestKey = key || crypto.randomUUID();
      setKey(requestKey);
      const result = value(
        await api.POST("/api/v1/invoices", {
          params: {
            header: { ...headers(token), "Idempotency-Key": requestKey },
          },
          body: { sample_id: sample },
        }),
      );
      window.location.hash = "auditor/" + result.invoice_id;
      refresh();
    });
  }
  function save() {
    if (!id || !detail) return;
    void action(async () => {
      const next = value(
        await api.PATCH("/api/v1/invoices/{id}/review", {
          params: { path: { id }, header: headers(token) },
          body: { version: detail.version, fields: edits, reason },
        }),
      );
      setDetail(next);
      setEdits({});
      setReason("");
      load();
    });
  }
  function decide(decision: "approve" | "reject") {
    if (!id || !detail) return;
    void action(async () => {
      value(
        await api.POST("/api/v1/invoices/{id}/decision", {
          params: { path: { id }, header: headers(token) },
          body: { version: detail.version, decision, reason },
        }),
      );
      setReason("");
      load();
    });
  }
  function retry() {
    if (!id || !detail) return;
    void action(async () => {
      const requestKey = key || crypto.randomUUID();
      setKey(requestKey);
      value(
        await api.POST("/api/v1/invoices/{id}/retry", {
          params: {
            path: { id },
            header: { ...headers(token), "Idempotency-Key": requestKey },
          },
          body: { version: detail.version },
        }),
      );
      setKey("");
      load();
    });
  }
  return (
    <div className="stack">
      {error && (
        <div role="alert" className="error">
          {error} {id && <button onClick={load}>Refresh invoice</button>}
        </div>
      )}
      {!id && !canWrite && <p>Your viewer role can inspect invoices in the <a href="#queue">review queue</a>.</p>}
      {!id && canWrite && (
        <section className="panel pad">
          <h2>Select a sample</h2>
          <div className="formrow">
            <label>
              Fictional invoice
              <select
                value={sample}
                onChange={(e) => {
                  choose(e.target.value);
                  setKey("");
                }}
              >
                {samples.map((s) => (
                  <option key={s.sample_id} value={s.sample_id}>
                    {s.title}
                  </option>
                ))}
              </select>
            </label>
            <button
              className="primary"
              disabled={busy || !chosen}
              onClick={submit}
            >
              {busy ? "Submitting…" : "Process invoice"}
            </button>
          </div>
        </section>
      )}
      {id && !detail && !error && <p role="status">Loading invoice…</p>}
      {detail && (
        <section className="panel pad">
          <div className="sectionhead">
            <div>
              <p className="eyebrow">Fictional invoice</p>
              <h2>{chosen?.title}</h2>
            </div>
            <State status={status} />
          </div>
          {["QUEUED", "PROCESSING"].includes(status) && (
            <p role="status">Waiting for the mock extraction worker…</p>
          )}
          {canWrite && status === "FAILED" && (
            <button disabled={busy} onClick={retry}>
              Retry extraction
            </button>
          )}
          {canWrite && <a href="#auditor">Select another sample</a>}
        </section>
      )}
      {chosen && (
        <details className="panel preview" open={!id}>
          <summary>Source document · {chosen.title}</summary>
          <object
            type="application/pdf"
            data={chosen.preview_url}
            aria-label="Fictional source PDF"
          >
            <a href={chosen.preview_url}>Open fictional PDF</a>
          </object>
          <a href={chosen.preview_url} target="_blank" rel="noreferrer">
            Open PDF in a new tab
          </a>
        </details>
      )}
      {detail?.revision && (
        <>
          <section className="panel pad">
            <div className="sectionhead">
              <h2>Validation</h2>
              <span className="meta">
                {detail.validation_results.length} issues
              </span>
            </div>
            {detail.validation_results.length ? (
              <ul className="issues">
                {detail.validation_results.map((r, i) => (
                  <li key={i}>
                    <strong>{r.rule_code.replaceAll("_", " ")}</strong>
                    <span>{r.message}</span>
                    <small>
                      Expected {r.expected} · Observed {r.actual}
                    </small>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="positive">
                All deterministic checks passed. Human approval is required.
              </p>
            )}
          </section>
          <section className="panel pad">
            <div className="sectionhead">
              <h2>Extracted fields</h2>
              <span className="meta">
                Mock · Revision {detail.invoice.revision_no}
              </span>
            </div>
            <div className="tablewrap">
              <table>
                <thead>
                  <tr>
                    <th>Field</th>
                    <th>Value</th>
                    <th>Source evidence</th>
                  </tr>
                </thead>
                <tbody>
                  {(
                    Object.keys(
                      detail.revision.fields,
                    ) as (keyof Model<"Fields">)[]
                  ).map((k) => {
                    const e = detail.revision!.evidence[k];
                    return (
                      <tr key={k}>
                        <th>{fieldLabel(k)}</th>
                        <td>
                          {editable ? (
                            <input
                              aria-label={fieldLabel(k)}
                              value={
                                k in edits
                                  ? (edits[k] ?? "")
                                  : (detail.revision!.fields[k] ?? "")
                              }
                              placeholder="Missing"
                              onChange={(event) =>
                                setEdits({
                                  ...edits,
                                  [k]: event.target.value || null,
                                })
                              }
                            />
                          ) : (
                            (detail.revision!.fields[k] ?? "Missing")
                          )}
                        </td>
                        <td>
                          {e ? (
                            <span>
                              p. {e.page} · {e.excerpt}
                            </span>
                          ) : (
                            "No source excerpt"
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </section>
        </>
      )}
      {editable && (
        <section className="panel pad">
          <h2>Review decision</h2>
          {!canApprove && <p>An owner or admin must approve unresolved findings. You can correct the fields and run the checks again.</p>}
          <label>
            Review reason
            <textarea
              maxLength={1000}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Record the reason for a correction, rejection or override"
            />
          </label>
          <div className="actions">
            <button
              disabled={busy || !reason.trim() || !Object.keys(edits).length}
              onClick={save}
            >
              Save correction
            </button>
            <button
              className="primary"
              disabled={
                busy ||
                !!Object.keys(edits).length ||
                !canApprove || (status === "PENDING_REVIEW" && !reason.trim())
              }
              onClick={() => decide("approve")}
            >
              {member ? "Approve as " + member.display_name : "Simulate approval"}
            </button>
            <button
              disabled={busy || !!Object.keys(edits).length || !reason.trim()}
              onClick={() => decide("reject")}
            >
              Reject invoice
            </button>
          </div>
        </section>
      )}
      {audit.length > 0 && (
        <section className="panel pad">
          <h2>Audit history</h2>
          {detail?.sandbox_notification && (
            <p className="meta">
              Sandbox notification · {statusText(detail.sandbox_notification)}{" "}
              <button onClick={load}>Refresh status</button>
            </p>
          )}
          <ol className="timeline">
            {audit.map((a) => (
              <li key={a.id}>
                <strong>{statusText(a.to_state)}</strong>
                <span>
                  {a.actor_user_id ? a.actor : a.actor.replaceAll("_", " ")} ·{" "}
                  {new Date(a.created_at).toLocaleTimeString()}
                </span>
                <p>{a.reason}</p>
              </li>
            ))}
          </ol>
          {id && cursor && (
            <button
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  const next = value(
                    await api.GET("/api/v1/invoices/{id}/audit", {
                      params: { path: { id }, query: { limit: 100, cursor } },
                    }),
                  );
                  setAudit((previous) => [...previous, ...next.items]);
                  setCursor(next.next_cursor);
                })
              }
            >
              Load more history
            </button>
          )}
        </section>
      )}
    </div>
  );
}
