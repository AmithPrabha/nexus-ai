"use client";
import { useState } from "react";
import { api, headers, Model, value } from "../lib/client";
const suggestions = [
  "Who approves purchases above AED 10,000?",
  "What are the travel booking deadlines?",
  "What is the current UAE corporate tax rate?",
];
export default function Policy({
  token,
  refresh,
}: {
  token: string;
  refresh: () => void;
}) {
  const [q, setQ] = useState(""),
    [result, setResult] = useState<Model<"Answer"> | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setResult(null);
    setBusy(true);
    try {
      setResult(
        value(
          await api.POST("/api/v1/policy/questions", {
            params: { header: headers(token) },
            body: { question: q.trim() },
          }),
        ),
      );
      refresh();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="stack">
      <form className="panel pad" onSubmit={submit}>
        <h2>Ask the handbook</h2>
        <label>
          Policy question
          <textarea
            value={q}
            maxLength={1000}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Ask about procurement, people or invoice controls"
            required
          />
        </label>
        <div className="suggestions">
          {suggestions.map((s) => (
            <button type="button" onClick={() => setQ(s)} key={s}>
              {s}
            </button>
          ))}
        </div>
        <button className="primary" disabled={busy || !q.trim()}>
          {busy ? "Searching…" : "Ask policy question"}
        </button>
      </form>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {result && (
        <section className="panel pad" aria-live="polite">
          <p className="eyebrow">
            {result.abstained ? "Insufficient evidence" : "Cited answer"}
          </p>
          <h2>{result.answer}</h2>
          <p className="meta">
            Mock retrieval · {result.telemetry.latency_ms} ms
          </p>
          {result.citations.map((c) => (
            <details key={c.chunk_id} className="citation">
              <summary>
                {c.title} · p. {c.page} · {c.clause}
              </summary>
              <blockquote>{c.excerpt}</blockquote>
              <small>Fictional policy · Version {c.version}</small>
            </details>
          ))}
        </section>
      )}
    </div>
  );
}
