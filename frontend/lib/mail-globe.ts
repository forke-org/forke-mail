type Vec = [number, number, number];
type Mail = { a: Vec; b: Vec; start: number; dur: number; height: number };

export function mountMailGlobe(canvas: HTMLCanvasElement, landB64: string): () => void {
  const raw = atob(landB64);
  const land = new Uint8Array(360 * 180);
  for (let i = 0; i < raw.length; i++) {
    const byte = raw.charCodeAt(i);
    for (let b = 0; b < 8; b++) land[i * 8 + b] = (byte >> (7 - b)) & 1;
  }
  const isLand = (lat: number, lon: number) => {
    let x = Math.floor(((lon + 180) % 360 + 360) % 360);
    let y = Math.floor(90 - lat);
    if (y < 0) y = 0; if (y > 179) y = 179;
    return land[y * 360 + x] === 1;
  };

  const CITIES: [number, number][] = [
    [19.08, 72.88], [28.61, 77.21], [12.97, 77.59], [1.35, 103.82], [35.68, 139.69], [37.57, 126.98],
    [22.32, 114.17], [-33.87, 151.21], [25.2, 55.27], [30.04, 31.24], [-1.29, 36.82], [6.52, 3.38],
    [-26.2, 28.05], [51.51, -0.13], [48.86, 2.35], [52.52, 13.4], [59.33, 18.07], [40.42, -3.7],
    [41.01, 28.98], [55.76, 37.62], [40.71, -74.01], [37.77, -122.42], [43.65, -79.38], [19.43, -99.13],
    [-23.55, -46.63], [-34.6, -58.38], [4.71, -74.07], [64.15, -21.94], [-36.85, 174.76], [14.6, 120.98],
  ];

  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;
  let W = 0, H = 0, dpr = 1, R = 0, CX = 0, CY = 0, CW = 8, CH = 13;
  const FONT = '"Geist Mono", ui-monospace, SFMono-Regular, Menlo, monospace';
  const RAMP = ' .,:;-=+*#%@';
  const TILT = -0.32;

  function resize() {
    const r = canvas.getBoundingClientRect();
    dpr = Math.min(2, window.devicePixelRatio || 1);
    W = r.width; H = r.height;
    canvas.width = Math.round(W * dpr); canvas.height = Math.round(H * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    R = Math.min(H * 0.49, W * 0.95);
    CX = 0; CY = H * 0.5;
    CW = Math.max(6, Math.round(R / 48));
    CH = Math.round(CW * 1.6);
  }

  const toVec = (lat: number, lon: number): Vec => {
    const a = (lat * Math.PI) / 180, b = (lon * Math.PI) / 180;
    return [Math.cos(a) * Math.sin(b), Math.sin(a), Math.cos(a) * Math.cos(b)];
  };
  function view(v: Vec, spin: number): Vec {
    const cs = Math.cos(spin), sn = Math.sin(spin);
    const x1 = v[0] * cs + v[2] * sn, z1 = -v[0] * sn + v[2] * cs, y1 = v[1];
    const ct = Math.cos(TILT), st = Math.sin(TILT);
    return [x1, y1 * ct - z1 * st, y1 * st + z1 * ct];
  }
  const project = (p: Vec, lift = 1): [number, number] => [CX + p[0] * R * lift, CY - p[1] * R * lift];

  function slerp(a: Vec, b: Vec, t: number): Vec {
    const d = Math.min(1, Math.max(-1, a[0] * b[0] + a[1] * b[1] + a[2] * b[2]));
    const w = Math.acos(d);
    if (w < 1e-4) return [a[0], a[1], a[2]];
    const s = Math.sin(w), k1 = Math.sin((1 - t) * w) / s, k2 = Math.sin(t * w) / s;
    return [a[0] * k1 + b[0] * k2, a[1] * k1 + b[1] * k2, a[2] * k1 + b[2] * k2];
  }

  const mails: Mail[] = [];
  const pulses: { v: Vec; start: number }[] = [];
  let spin = 0.9;
  function frontFacing(v: Vec) { const p = view(v, spin); return p[2] > 0.15 && p[0] > -0.05; }
  function spawn(now: number) {
    for (let tries = 0; tries < 40; tries++) {
      const a = CITIES[Math.floor(Math.random() * CITIES.length)];
      const b = CITIES[Math.floor(Math.random() * CITIES.length)];
      if (a === b) continue;
      const va = toVec(a[0], a[1]), vb = toVec(b[0], b[1]);
      const dist = Math.acos(Math.min(1, va[0] * vb[0] + va[1] * vb[1] + va[2] * vb[2]));
      if (dist < 0.35 || dist > 1.9) continue;
      if (!frontFacing(va)) continue;
      mails.push({ a: va, b: vb, start: now, dur: 2200 + dist * 1400, height: 0.12 + dist * 0.12 });
      return;
    }
  }

  function drawGlobe() {
    ctx.font = `${Math.round(CH * 0.82)}px ${FONT}`;
    ctx.textBaseline = 'middle';
    ctx.textAlign = 'center';
    const L = [0.55, 0.45, 0.7];
    const ll = Math.hypot(L[0], L[1], L[2]); L[0] /= ll; L[1] /= ll; L[2] /= ll;
    const ct = Math.cos(-TILT), st = Math.sin(-TILT), cs = Math.cos(-spin), sn = Math.sin(-spin);
    const cols = Math.ceil((R + CW) / CW), rows = Math.ceil((2 * R) / CH) + 1;
    for (let j = 0; j < rows; j++) {
      const y = CY - R + j * CH + CH / 2;
      const ny = (CY - y) / R;
      for (let i = 0; i < cols; i++) {
        const x = CX + i * CW + CW / 2;
        const nx = (x - CX) / R;
        const r2 = nx * nx + ny * ny;
        if (r2 > 1) continue;
        const nz = Math.sqrt(1 - r2);
        const y1 = ny * ct - nz * st, z1 = ny * st + nz * ct;
        const x0 = nx * cs + z1 * sn, z0 = -nx * sn + z1 * cs;
        const lat = (Math.asin(Math.max(-1, Math.min(1, y1))) * 180) / Math.PI;
        const lon = (Math.atan2(x0, z0) * 180) / Math.PI;
        const light = Math.max(0, nx * L[0] + ny * L[1] + nz * L[2]);
        const rim = Math.pow(1 - nz, 2.2);
        if (isLand(lat, lon)) {
          const coast = !isLand(lat + 1.6, lon) || !isLand(lat - 1.6, lon) || !isLand(lat, lon + 1.6) || !isLand(lat, lon - 1.6);
          const grain = (Math.sin(lat * 12.9898 + lon * 78.233) * 43758.5453) % 1;
          const v = Math.min(1, 0.15 + light * 0.85 + Math.abs(grain) * 0.12);
          const ch = RAMP[Math.min(RAMP.length - 1, 3 + Math.floor(v * (RAMP.length - 4)))];
          ctx.fillStyle = coast ? `rgba(255,163,71,${(0.35 + v * 0.6).toFixed(3)})` : `rgba(240,227,222,${(0.2 + v * 0.72).toFixed(3)})`;
          ctx.fillText(coast ? '*' : ch, x, y);
        } else if ((i + j) % 2 === 0) {
          const wave = 0.5 + 0.5 * Math.sin(lon * 0.18 + lat * 0.12 + spin * 40);
          const a = 0.04 + light * 0.1 + wave * 0.05 + rim * 0.32;
          ctx.fillStyle = rim > 0.25 ? `rgba(255,122,0,${(a * 0.9).toFixed(3)})` : `rgba(240,227,222,${a.toFixed(3)})`;
          ctx.fillText(rim > 0.5 ? ':' : wave > 0.8 ? '-' : '.', x, y);
        }
        if (Math.abs(lat % 30) < 0.9 || Math.abs(lon % 30) < 0.9) {
          ctx.fillStyle = `rgba(255,122,0,${(0.05 + light * 0.08).toFixed(3)})`;
          ctx.fillRect(x - 0.5, y - 0.5, 1, 1);
        }
      }
    }
  }

  function drawAtmosphere() {
    const g = ctx.createRadialGradient(CX, CY, R * 0.92, CX, CY, R * 1.22);
    g.addColorStop(0, 'rgba(255,122,0,0)');
    g.addColorStop(0.35, 'rgba(255,122,0,0.10)');
    g.addColorStop(1, 'rgba(255,122,0,0)');
    ctx.fillStyle = g;
    ctx.beginPath(); ctx.arc(CX, CY, R * 1.22, 0, Math.PI * 2); ctx.fill();
    ctx.strokeStyle = 'rgba(255,163,71,0.35)';
    ctx.lineWidth = 1;
    ctx.beginPath(); ctx.arc(CX, CY, R + 0.5, -Math.PI / 2, Math.PI / 2); ctx.stroke();
  }

  function envelope(x: number, y: number, angle: number, size: number, alpha: number) {
    ctx.save();
    ctx.translate(x, y); ctx.rotate(angle);
    const w = size, h = size * 0.68;
    ctx.fillStyle = `rgba(21,20,20,${alpha})`;
    ctx.strokeStyle = `rgba(255,163,71,${alpha})`;
    ctx.lineWidth = 1.1;
    ctx.beginPath(); ctx.rect(-w / 2, -h / 2, w, h); ctx.fill(); ctx.stroke();
    ctx.beginPath(); ctx.moveTo(-w / 2, -h / 2); ctx.lineTo(0, h * 0.1); ctx.lineTo(w / 2, -h / 2); ctx.stroke();
    ctx.restore();
  }

  function drawMails(now: number) {
    for (let m = mails.length - 1; m >= 0; m--) {
      const mail = mails[m];
      const t = (now - mail.start) / mail.dur;
      if (t >= 1) {
        pulses.push({ v: mail.b, start: now });
        mails.splice(m, 1);
        continue;
      }
      const head = Math.min(1, t * 1.15);
      const tail = Math.max(0, head - 0.42);
      const STEPS = 48;
      let prev: { x: number; y: number; hidden: boolean } | null = null;
      for (let s = 0; s <= STEPS; s++) {
        const u = tail + ((head - tail) * s) / STEPS;
        const p = view(slerp(mail.a, mail.b, u), spin);
        const lift = 1 + Math.sin(Math.PI * u) * mail.height;
        const pt = project(p, lift);
        const hidden = p[2] < 0 && Math.hypot(p[0] * lift, p[1] * lift) < 1;
        if (prev && !hidden && !prev.hidden) {
          const k = s / STEPS;
          ctx.strokeStyle = `rgba(255,122,0,${(k * 0.85).toFixed(3)})`;
          ctx.lineWidth = 0.6 + k * 1.2;
          ctx.beginPath(); ctx.moveTo(prev.x, prev.y); ctx.lineTo(pt[0], pt[1]); ctx.stroke();
        }
        prev = { x: pt[0], y: pt[1], hidden };
      }
      const hp = view(slerp(mail.a, mail.b, head), spin);
      const hl = 1 + Math.sin(Math.PI * head) * mail.height;
      const ahead = view(slerp(mail.a, mail.b, Math.min(1, head + 0.02)), spin);
      const p1 = project(hp, hl), p2 = project(ahead, 1 + Math.sin(Math.PI * Math.min(1, head + 0.02)) * mail.height);
      const visible = !(hp[2] < 0 && Math.hypot(hp[0] * hl, hp[1] * hl) < 1);
      const fade = Math.min(1, t * 6, (1 - t) * 8);
      if (visible) envelope(p1[0], p1[1], Math.atan2(p2[1] - p1[1], p2[0] - p1[0]), Math.max(9, R / 26), fade);
      if (t < 0.15) {
        const sp = project(view(mail.a, spin));
        ctx.fillStyle = `rgba(255,163,71,${(1 - t / 0.15) * 0.9})`;
        ctx.beginPath(); ctx.arc(sp[0], sp[1], 2, 0, Math.PI * 2); ctx.fill();
      }
    }
    for (let i = pulses.length - 1; i >= 0; i--) {
      const pl = pulses[i];
      const k = (now - pl.start) / 1100;
      if (k >= 1) { pulses.splice(i, 1); continue; }
      const p = view(pl.v, spin);
      if (p[2] < 0) continue;
      const [x, y] = project(p);
      const rad = 3 + k * R * 0.07;
      ctx.strokeStyle = `rgba(255,122,0,${(1 - k) * 0.9})`;
      ctx.lineWidth = 1.2;
      ctx.beginPath(); ctx.ellipse(x, y, rad, rad * Math.max(0.35, p[2]), 0, 0, Math.PI * 2); ctx.stroke();
      ctx.fillStyle = `rgba(255,163,71,${1 - k})`;
      ctx.beginPath(); ctx.arc(x, y, 2.2, 0, Math.PI * 2); ctx.fill();
    }
  }

  function drawStars(now: number) {
    for (let i = 0; i < 70; i++) {
      const x = ((i * 9301 + 49297) % 233280) / 233280 * W;
      const y = ((i * 49297 + 9301) % 233280) / 233280 * H;
      if (Math.hypot(x - CX, y - CY) < R * 1.25) continue;
      const tw = 0.08 + 0.12 * (0.5 + 0.5 * Math.sin(now / 900 + i));
      ctx.fillStyle = `rgba(240,227,222,${tw.toFixed(3)})`;
      ctx.fillRect(x, y, 1, 1);
    }
  }

  resize();
  window.addEventListener('resize', resize);
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  let raf = 0, nextSpawn = 0;
  function frame(now: number) {
    if (!reduce) spin = 0.9 + now * 0.00006;
    if (now >= nextSpawn && mails.length < 9) { spawn(now); nextSpawn = now + 320 + Math.random() * 420; }
    ctx.clearRect(0, 0, W, H);
    drawStars(now);
    drawAtmosphere();
    drawGlobe();
    drawMails(now);
    raf = requestAnimationFrame(frame);
  }
  raf = requestAnimationFrame(frame);
  return () => { cancelAnimationFrame(raf); window.removeEventListener('resize', resize); };
}
