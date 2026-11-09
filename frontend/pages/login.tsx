"use client";
import Link from "next/link";
import { useState } from "react";
import AuthShell, { GoogleIcon, GitHubIcon, MailIcon, LockIcon } from "@/components/auth/AuthShell";
import s from "@/components/auth/auth.module.css";

export default function LoginPage() {
  const [id, setId] = useState("");
  const [pw, setPw] = useState("");
  const [show, setShow] = useState(false);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    // TODO: call your auth handler with { id, pw }
  };

  return (
    <AuthShell compact>
      <h1 className={s.title}>Welcome back to <em>Forke</em></h1>
      <p className={s.sub}>Sign in to continue to your workspace.</p>

      <div className={s.social}>
        <button type="button" className={s.btn}><GoogleIcon /> Google</button>
        <button type="button" className={s.btn}><GitHubIcon /> GitHub</button>
      </div>
      <div className={s.divider}>or</div>

      <form onSubmit={submit}>
        <label className={s.field}>
          <span>Email or username</span>
          <div className={s.control}>
            <i className={s.icon}><MailIcon /></i>
            <input className={`${s.input} ${s.withIcon}`} value={id} onChange={(e) => setId(e.target.value)} placeholder="name@example.com" autoComplete="username" required />
          </div>
        </label>
        <label className={s.field}>
          <span>Password</span>
          <div className={`${s.control} ${s.pw}`}>
            <i className={s.icon}><LockIcon /></i>
            <input className={`${s.input} ${s.withIcon}`} type={show ? "text" : "password"} value={pw} onChange={(e) => setPw(e.target.value)} placeholder="Enter your password" autoComplete="current-password" required />
            <button type="button" className={s.eye} onClick={() => setShow(!show)} aria-label={show ? "Hide password" : "Show password"}>
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z" /><circle cx="12" cy="12" r="3" />{show && <path d="M3 3l18 18" />}</svg>
            </button>
          </div>
        </label>
        <Link href="/forgot-password" className={s.forgot}>Forgot password?</Link>
        <button className={s.primary} type="submit">Sign in</button>
      </form>

      <p className={s.swap}>New to Forke? <Link href="/login">Join the movement</Link></p>
    </AuthShell>
  );
}
