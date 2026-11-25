"use client";
import { useCallback, useEffect, useState } from "react";
import { api, headers, Model, value } from "../lib/client";
import Auditor, { State } from "./invoice";
import Policy from "./policy";
import SignIn from "./sign-in";
import Link from "next/link";

function openOrganizationSignIn() {
  if (location.pathname === "/organization") location.reload();
  else location.replace("/organization");
}

const views = {
  overview: "Overview",
  auditor: "Invoice auditor",
  queue: "Review queue",
  policy: "Policy assistant",
  evaluation: "Evaluation",
  architecture: "Architecture",
};
type View = keyof typeof views;
const glyphs: Record<View, string> = {
  overview: "▦",
  auditor: "▤",
  queue: "≡",
  policy: "¶",
  evaluation: "▥",
  architecture: "⌘",
};
type Report = { status: string; [key: string]: unknown };
function InvoiceList({
  items,
  samples,
}: {
  items: Model<"Invoice">[];
  samples: Model<"Sample">[];
}) {
  if (!items.length)
    return (
      <div className="empty">
        <strong>No invoices here yet</strong>
        <p>Select a fictional sample to start a review.</p>
        <a href="#auditor">Open invoice auditor</a>
      </div>
    );
  return (
    <div className="tablewrap">
      <table>
        <thead>
          <tr>
            <th>Sample</th>
            <th>State</th>
            <th>Created</th>
          </tr>
        </thead>
        <tbody>
          {items.map((i) => (
            <tr key={i.invoice_id}>
              <td>
                <a href={"#auditor/" + i.invoice_id}>
                  {samples.find((s) => s.sample_id === i.sample_id)?.title ??
                    i.sample_id}
                </a>
              </td>
              <td>
                <State status={i.status} />
              </td>
              <td>{new Date(i.created_at).toLocaleDateString()}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
export default function Workspace() {
  const [member, setMember] = useState<Model<"Member"> | null>(null);
  const [needsLogin, setNeedsLogin] = useState(false);
  const [view, setView] = useState<View>("overview"),
    [id, setID] = useState<string>();
  const [token, setToken] = useState(""),
    [samples, setSamples] = useState<Model<"Sample">[]>([]);
  const [metrics, setMetrics] = useState<Model<"Metrics"> | null>(null),
    [invoices, setInvoices] = useState<Model<"Invoice">[]>([]);
  const [error, setError] = useState(""),
    [filter, setFilter] = useState(""),
    [report, setReport] = useState<Report | null>(null);
  const [sessionVersion, renew] = useState(0);
  const refresh = useCallback(async () => {
    try {
      const [m, i] = await Promise.all([
        api.GET("/api/v1/metrics/demo"),
        api.GET("/api/v1/invoices", { params: { query: { limit: 100 } } }),
      ]);
      setMetrics(value(m));
      let page = value(i);
      const items = [...page.items];
      while (page.next_cursor) {
        page = value(
          await api.GET("/api/v1/invoices", {
            params: { query: { limit: 100, cursor: page.next_cursor } },
          }),
        );
        items.push(...page.items);
      }
      setInvoices(
        items.sort((a, b) => b.created_at.localeCompare(a.created_at)),
      );
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    const navigate = () => {
      const [name, invoice] = window.location.hash.slice(1).split("/");
      setView(Object.hasOwn(views, name) ? (name as View) : "overview");
      setID(invoice);
      setError("");
    };
    navigate();
    window.addEventListener("hashchange", navigate);
    return () => window.removeEventListener("hashchange", navigate);
  }, []);
  useEffect(() => {
    let live = true;
    async function open() {
      try {
        const identity = await api.GET("/api/v1/auth/session");
        if (!live) return;
        if (identity.error && identity.response.status !== 401) throw new Error(identity.error.error.message);
        if (!identity.data && location.pathname === "/organization") {
          setNeedsLogin(true);
          return;
        }
        const s = identity.data ?? value(await api.POST("/api/v1/demo/sessions", {
          params: { header: { Origin: location.origin } },
        }));
        setMember(identity.data?.member ?? null);
        const all = value(await api.GET("/api/v1/samples"));
        if (!live) return;
        setToken(s.csrf_token);
        setSamples(all.items);
        await refresh();
      } catch (e) {
        if (live) setError((e as Error).message);
      }
    }
    void open();
    return () => {
      live = false;
    };
  }, [sessionVersion, refresh]);
  useEffect(() => {
    if (view === "evaluation" && token) {
      api
        .GET("/api/v1/evaluation")
        .then((r) => setReport(value(r)))
        .catch((e) => setError(e.message));
    }
    if ((view === "queue" || view === "overview") && token) {
      const timer = window.setInterval(refresh, 5000);
      return () => window.clearInterval(timer);
    }
  }, [view, token, refresh]);
  useEffect(() => {
    if (!member) return;
    const expired = openOrganizationSignIn;
    window.addEventListener("nexus-session-ended", expired);
    return () => window.removeEventListener("nexus-session-ended", expired);
  }, [member]);
  async function reset() {
    try {
      if (member) {
        value(await api.POST("/api/v1/auth/logout", { params: { header: headers(token) } }));
        openOrganizationSignIn();
        return;
      }
      value(
        await api.POST("/api/v1/demo/reset", {
          params: { header: headers(token) },
        }),
      );
      setToken("");
      setInvoices([]);
      setMetrics(null);
      setReport(null);
      setSamples([]);
      location.hash = "overview";
      renew((n) => n + 1);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  return (
    <div className="shell">
      <a
        className="skip"
        href="#main"
        onClick={(e) => {
          e.preventDefault();
          document.getElementById("main")?.focus();
        }}
      >
        Skip to workspace
      </a>
      <aside>
        <a className="brand" href="#overview">
          <span className="mark" aria-hidden>
            N
          </span>
          NexusAI
        </a>
        <p className="navlabel">Workspace</p>
        <nav>
          {(Object.keys(views) as View[]).filter(v => member?.role !== "VIEWER" || v !== "auditor").map((v) => (
            <a
              key={v}
              aria-current={v === view ? "page" : undefined}
              href={"#" + v}
            >
              <span aria-hidden>{glyphs[v]}</span>
              {views[v]}
            </a>
          ))}
        </nav>
        <div className="sidefoot">
          <b>{member?.tenant_name || (needsLogin ? "Organization" : "Demo session")}</b>
          <small>{member ? member.display_name + " · " + member.role : needsLogin ? "Sign-in required" : "Isolated · 24-hour expiry"}</small>
          {!needsLogin && <button onClick={reset} disabled={!token}>
            {member ? "Sign out" : "Reset demo"}
          </button>}
          {!member && !needsLogin && <Link href="/organization">Organization sign-in</Link>}
        </div>
      </aside>
      <main id="main" tabIndex={-1}>
        <header>
          <span>
            NexusAI <span className="muted">/</span> Workspace
          </span>
          <span className="mock">Mock provider</span>
        </header>
        <div className="content">
          <div className="pagetitle">
            <p className="eyebrow">Document intelligence</p>
            <h1>{view === "overview" ? "Workspace overview" : views[view]}</h1>
          </div>
          {error && (
            <p role="alert" className="error">
              {error}{" "}
              <button
                onClick={() => {
                  setError("");
                  renew((n) => n + 1);
                }}
              >
                Reconnect
              </button>
            </p>
          )}
          {needsLogin && <SignIn />}
          {!needsLogin && !token && !error && <p role="status">Opening your demo session…</p>}
          {token && view === "overview" && (
            <div className="stack">
              <section className="hero">
                <div>
                  <span className="heroLabel">Sample-only workspace</span>
                  <h2>Invoice review desk</h2>
                  <p>
                    Process a fictional UAE invoice, inspect its evidence and
                    review its financial checks.
                  </p>
                  <a className="lightbutton" href={member?.role === "VIEWER" ? "#queue" : "#auditor"}>
                    {member?.role === "VIEWER" ? "View invoices" : "Review an invoice"}
                  </a>
                </div>
                <div className="paper" aria-hidden>
                  <div className="fold" />
                  <span>FICTIONAL INVOICE</span>
                  <strong>NX-0001</strong>
                  <i />
                  <i />
                  <i />
                  <small>AED</small>
                </div>
              </section>
              <div className="metrics">
                {[
                  ["Invoices", metrics?.invoices],
                  ["Awaiting review", metrics?.awaiting_review],
                  ["Mock runs", metrics?.runs],
                  ["Failed runs", metrics?.failed_runs],
                ].map(([k, n]) => (
                  <div key={k}>
                    <span>{k}</span>
                    <strong>{n ?? "—"}</strong>
                  </div>
                ))}
              </div>
              <section className="panel pad">
                <div className="sectionhead">
                  <h2>Recent invoices</h2>
                  <a href="#queue">View queue</a>
                </div>
                <InvoiceList items={invoices.slice(0, 5)} samples={samples} />
              </section>
              <section className="panel pad">
                <h2>Session telemetry</h2>
                <p>
                  Average operation time{" "}
                  <strong>
                    {metrics?.average_operation_ms == null
                      ? "Unavailable"
                      : Math.round(metrics.average_operation_ms) + " ms"}
                  </strong>{" "}
                  · Sandbox notifications{" "}
                  <strong>{metrics?.sandbox_notifications ?? "—"}</strong>
                </p>
                <p className="meta">
                  Token usage and estimated cost are unavailable in mock mode.
                </p>
              </section>
            </div>
          )}
          {token && view === "auditor" && (
            <Auditor
              key={id || "new"}
              id={id}
              samples={samples}
              token={token}
              member={member}
              refresh={refresh}
            />
          )}
          {token && view === "policy" && (
            <Policy token={token} refresh={refresh} />
          )}
          {token && view === "queue" && (
            <section className="panel pad">
              <div className="sectionhead">
                <h2>Workspace invoices</h2>
                <label>
                  Filter by state
                  <select
                    value={filter}
                    onChange={(e) => setFilter(e.target.value)}
                  >
                    <option value="">All states</option>
                    {[
                      "QUEUED",
                      "PROCESSING",
                      "PENDING_REVIEW",
                      "READY_FOR_APPROVAL",
                      "APPROVED",
                      "REJECTED",
                      "FAILED",
                    ].map((s) => (
                      <option key={s}>{s}</option>
                    ))}
                  </select>
                </label>
              </div>
              <InvoiceList
                items={invoices.filter((i) => !filter || i.status === filter)}
                samples={samples}
              />
            </section>
          )}
          {token && view === "evaluation" && (
            <section className="panel pad">
              <h2>Mock benchmark</h2>
              {!report ? (
                <p>Loading report…</p>
              ) : report.status === "not_run" ? (
                <p>The benchmark has not run yet.</p>
              ) : (
                <>
                  <p>
                    Version {String(report.dataset)} · {String(report.date)} ·{" "}
                    {String(report.mode)} mode
                  </p>
                  <div className="report">
                    {Object.entries(
                      (report.metrics || {}) as Record<string, unknown>,
                    ).map(([k, v]) => (
                      <div key={k}>
                        <span>{k.replaceAll("_", " ")}</span>
                        <strong>{v == null ? "Unavailable" : String(v)}</strong>
                      </div>
                    ))}
                  </div>
                  <p>{String(report.limitations)}</p>
                  <a href="/api/v1/evaluation">Open JSON report</a>
                </>
              )}
            </section>
          )}
          {view === "architecture" && (
            <section className="panel pad">
              <h2>From sample to decision</h2>
              <ol className="timeline">
                {[
                  [
                    "Next.js workspace",
                    "The browser selects an allowlisted PDF and submits a session-scoped request.",
                  ],
                  [
                    "Go gateway and PostgreSQL",
                    "Invoice, job and outbox are written in one transaction. Go owns validation, revisions and human decisions.",
                  ],
                  [
                    "Redis Streams and Python",
                    "A leased worker reads PDF page text, checks mock evidence and submits a fenced result.",
                  ],
                  [
                    "Policy retrieval",
                    "PostgreSQL full-text and vector search select assigned clauses. The gateway verifies the quoted source.",
                  ],
                ].map(([k, v]) => (
                  <li key={k}>
                    <strong>{k}</strong>
                    <p>{v}</p>
                  </li>
                ))}
              </ol>
              <div className="actions">
                <a href="/openapi.json">Public API contract</a>
                <a href="https://github.com/AmithPrabha/nexus-ai">
                  Source repository
                </a>
              </div>
            </section>
          )}
        </div>
      </main>
    </div>
  );
}
