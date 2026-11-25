"use client";
import { useState } from "react";
import Link from "next/link";
import { api } from "../lib/client";

export default function SignIn() {
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <form className="panel pad" onSubmit={async (event) => {
      event.preventDefault();
      setBusy(true);
      setError("");
      try {
        const result = await api.POST("/api/v1/auth/login", {
          params: { header: { Origin: location.origin } }, body: { code: code.trim() },
        });
        if (result.error) {
          setError(result.response.status === 401 ? "This code is invalid, expired or already used." : result.error.error.message);
          return;
        }
        location.hash = "overview";
        location.reload();
      } catch {
        setError("Sign-in is unavailable. Please try again.");
      } finally {
        setCode("");
        setBusy(false);
      }
    }}>
      <h2>Organization sign-in</h2>
      <label>One-time sign-in code
        <input type="password" autoComplete="one-time-code" required value={code}
          onChange={(event) => setCode(event.target.value)} />
      </label>
      <p>Use the code issued for your organization. It expires after ten minutes.</p>
      {error && <p role="alert">{error}</p>}
      <button disabled={busy || !code.trim()}>Sign in</button>
      <p><Link href="/">Open the public demo</Link></p>
    </form>
  );
}
