"use client";
import { useEffect, useRef } from "react";
import Image from "next/image";
import Link from "next/link";
import { Inter, Instrument_Serif } from "next/font/google";
import s from "./auth.module.css";

const inter = Inter({ subsets: ["latin"], variable: "--font-ui" });
const serif = Instrument_Serif({ subsets: ["latin"], weight: "400", style: ["normal", "italic"], variable: "--font-serif" });

export const GoogleIcon = () => (
  <svg width="18" height="18" viewBox="0 0 48 48" aria-hidden>
    <path fill="#EA4335" d="M24 9.5c3.5 0 6.6 1.2 9 3.6l6.7-6.7C35.6 2.4 30.2 0 24 0 14.6 0 6.5 5.4 2.6 13.2l7.8 6.1C12.3 13.6 17.7 9.5 24 9.5z" />
    <path fill="#4285F4" d="M46.5 24.5c0-1.6-.1-3.1-.4-4.5H24v8.5h12.7c-.6 3-2.3 5.5-4.8 7.2l7.5 5.8c4.4-4.1 7.1-10.1 7.1-17z" />
    <path fill="#FBBC05" d="M10.4 28.7A14.5 14.5 0 0 1 9.5 24c0-1.6.3-3.2.9-4.7l-7.8-6.1A24 24 0 0 0 0 24c0 3.9.9 7.5 2.6 10.8l7.8-6.1z" />
    <path fill="#34A853" d="M24 48c6.5 0 11.9-2.1 15.9-5.8l-7.5-5.8c-2.1 1.4-4.9 2.3-8.4 2.3-6.3 0-11.7-4.1-13.6-9.8l-7.8 6.1C6.5 42.6 14.6 48 24 48z" />
  </svg>
);
export const GitHubIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
    <path d="M12 .5a11.5 11.5 0 0 0-3.64 22.41c.58.1.79-.25.79-.56v-2c-3.2.7-3.88-1.37-3.88-1.37-.53-1.34-1.3-1.7-1.3-1.7-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.2 1.77 1.2 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.28 1.19-3.09-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.17 1.18a11 11 0 0 1 5.77 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.83 1.19 3.09 0 4.42-2.7 5.4-5.26 5.68.41.36.78 1.06.78 2.14v3.17c0 .31.21.67.8.56A11.5 11.5 0 0 0 12 .5z" />
  </svg>
);

const stroke = { fill: "none", stroke: "currentColor", strokeWidth: 1.6, strokeLinecap: "round", strokeLinejoin: "round" } as const;
export const MailIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" {...stroke} aria-hidden><rect x="3" y="5" width="18" height="14" rx="2.5" /><path d="m4 7 8 6 8-6" /></svg>
);
export const LockIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" {...stroke} aria-hidden><rect x="4.5" y="10.5" width="15" height="10" rx="2.5" /><path d="M8 10.5V8a4 4 0 0 1 8 0v2.5" /></svg>
);
export const UserIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" {...stroke} aria-hidden><circle cx="12" cy="8.5" r="3.5" /><path d="M5 20c.8-3.4 3.6-5.2 7-5.2s6.2 1.8 7 5.2" /></svg>
);

export default function AuthShell({ children, wide = false, compact = false }: { children: React.ReactNode; wide?: boolean; compact?: boolean }) {
  const page = useRef<HTMLElement>(null);
  const card = useRef<HTMLElement>(null);

  // The big light drifts on its own (it never follows the cursor); CSS variables only, no React re-renders.
  useEffect(() => {
    const el = page.current, c = card.current;
    if (!el || !c) return;
    const calm = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    let x = window.innerWidth / 2, y = window.innerHeight * 0.4, raf = 0;

    const frame = (t: number) => {
      const w = window.innerWidth, h = window.innerHeight;
      // on narrow screens the card covers most of the page, so let the light roam further to stay visible
      const narrow = w < 600;
      const tx = w * (0.5 + (calm ? 0 : narrow ? 0.34 : 0.3) * Math.sin(t / 3100));
      const ty = h * (0.45 + (calm ? 0 : narrow ? 0.36 : 0.2) * Math.sin(t / 2300 + 1));
      const k = calm ? 1 : 0.04;
      x += (tx - x) * k; y += (ty - y) * k;
      const r = c.getBoundingClientRect();
      el.style.setProperty("--mx", x.toFixed(1) + "px");
      el.style.setProperty("--my", y.toFixed(1) + "px");
      c.style.setProperty("--cx", (x - r.left).toFixed(1) + "px");
      c.style.setProperty("--cy", (y - r.top).toFixed(1) + "px");
      raf = requestAnimationFrame(frame);
    };
    raf = requestAnimationFrame(frame);
    return () => cancelAnimationFrame(raf);
  }, []);

  return (
    <main ref={page} className={`${s.page} ${inter.variable} ${serif.variable}`}>
      <div className={s.bg} aria-hidden>
        <div className={s.horizon} />
        <div className={s.dots} />
        <div className={s.dotsLit} />
        <div className={s.light} />
        <div className={s.grain} />
        <div className={s.vignette} />
      </div>

      <section ref={card} className={`${s.card} ${wide ? s.wide : ""} ${compact ? s.compact : ""}`}>
        <Link href="/" className={s.logoLink} aria-label="Forke home">
          <Image src="/forke-logo.png" alt="" width={44} height={44} className={s.logo} priority />
        </Link>
        {children}
      </section>

      <footer className={s.footer}>
        <p>The developer marketplace, reimagined.</p>
        <nav>
          <Link href="/">Home</Link>
          <Link href="/terms">Terms</Link>
          <Link href="/privacy">Privacy</Link>
          <span>© 2026 Forke</span>
        </nav>
      </footer>

    </main>
  );
}
