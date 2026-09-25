import { useState, type FormEvent } from "react";
import { api, ApiError, type User } from "../api";

export default function Login({ onSignedIn }: { onSignedIn: (u: User) => void }) {
  const [email, setEmail] = useState("reviewer@casereview.test");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      onSignedIn(await api.login(email, password));
    } catch (err) {
      setError(err instanceof ApiError && err.status === 401 ? "Email or password is incorrect. Check both and try again." : `${err instanceof Error ? err.message : "Sign-in failed"}. Check your connection and try again.`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="login-shell">
      <form className="card login-card" onSubmit={submit}>
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none"><path d="M3 12c3-4 6-4 9 0s6 4 9 0" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" /></svg>
          </span>
          Case Review
        </div>
        <h1>Sign in to review cases</h1>
        <label className="form-label">Email
          <input id="email" className="input" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </label>
        <label className="form-label">Password
          <input id="password" className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </label>
        {error && <p className="form-error" role="alert">{error}</p>}
        <button className="btn btn-primary" type="submit" disabled={busy}>{busy ? "Signing in…" : "Sign in"}</button>
      </form>
    </main>
  );
}
