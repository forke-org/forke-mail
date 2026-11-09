"use client";
import { useId, useState } from "react";
import { LockIcon } from "./AuthShell";
import s from "./auth.module.css";

const WORDS = ["", "Weak", "Fair", "Good", "Strong"];

/** 0 = too short, 1–4 = weak → strong */
export function strength(pw: string) {
  if (pw.length < 8) return 0;
  let n = 1;
  if (/[a-z]/.test(pw) && /[A-Z]/.test(pw)) n++;
  if (/\d/.test(pw)) n++;
  if (/[^A-Za-z0-9]/.test(pw)) n++;
  return n;
}

type Props = {
  label: string;
  name?: string;
  value: string;
  onChange: (v: string) => void;
  autoComplete: string;
  placeholder?: string;
  meter?: boolean;
  action?: React.ReactNode;
};

export default function PasswordField({ label, name = "password", value, onChange, autoComplete, placeholder, meter = false, action }: Props) {
  const id = useId();
  const [show, setShow] = useState(false);
  const n = meter ? strength(value) : 0;

  return (
    <div className={s.field}>
      <div className={s.labelRow}>
        <label htmlFor={id}>{label}</label>
        {action}
      </div>
      <div className={`${s.control} ${s.pw}`}>
        <i className={s.icon}><LockIcon /></i>
        <input
          id={id} name={name} className={`${s.input} ${s.withIcon}`} type={show ? "text" : "password"}
          value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder}
          autoComplete={autoComplete} minLength={meter ? 8 : undefined} required
        />
        <button type="button" className={s.eye} onClick={() => setShow(!show)} aria-label={show ? "Hide password" : "Show password"} aria-pressed={show}>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z" /><circle cx="12" cy="12" r="3" />{show && <path d="M3 3l18 18" />}</svg>
        </button>
      </div>
      {meter && (
        <>
          <div className={s.meter} data-level={n} aria-hidden>
            {[0, 1, 2, 3].map((i) => <i key={i} data-on={n > i} />)}
          </div>
          <p className={s.hint} aria-live="polite">
            {value.length === 0 ? "Use 8 or more characters." : n === 0 ? "Use at least 8 characters." : <>Strength: <b>{WORDS[n]}</b></>}
          </p>
        </>
      )}
    </div>
  );
}
