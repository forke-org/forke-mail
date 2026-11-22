import { KeyRound, Mail } from 'lucide-react';
import { api } from '../lib/api';
import { useSession } from '../lib/session';
import { useLoad } from '../lib/useLoad';
import { ButtonLink, Callout, Card, CopyButton, Loading } from './ui';

type Smtp = { enabled: boolean; host: string; username: string; starttls_port: string; tls_port: string };

function Row({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[13px] font-medium text-ink-2">{label}</span>
      <div className="flex items-center gap-2">
        <code className="flex h-9 min-w-0 flex-1 items-center truncate rounded-[6px] bg-surface-2 px-3 font-mono text-[13px] text-ink shadow-[inset_0_0_0_1px_var(--line)]">{value}</code>
        <CopyButton value={value} boxed />
      </div>
      <span className="text-xs text-muted">{hint}</span>
    </div>
  );
}

export default function SmtpSettings() {
  const { superAdmin } = useSession();
  const [state] = useLoad(() => api<Smtp>('GET', '/api/settings/smtp'), []);
  if (!state.data) return <Loading />;
  const s = state.data;
  return (
    <div className="flex flex-col gap-5">
      {!s.enabled ? (
        <Callout tone="warn" title="SMTP isn't switched on yet">The settings below are what you'll use once it is. Until then, send with the API.</Callout>
      ) : null}
      <Card title="SMTP server" icon={Mail} description="Send through any app or library that speaks SMTP: WordPress, Laravel, Django, Nodemailer, Outlook or Thunderbird. Mail goes through the same checks as the API: verified sender domain, suppressions, limits and tracking.">
        <div className="flex flex-col gap-4">
          <Row label="Host" value={s.host} hint="Use this hostname in your mail settings." />
          <div className="grid gap-4 sm:grid-cols-2">
            <Row label="Port (STARTTLS)" value={s.starttls_port || '587'} hint="Recommended. The connection is upgraded to TLS." />
            <Row label="Port (SSL/TLS)" value={s.tls_port || '465'} hint="Encrypted from the first byte." />
          </div>
          <Row label="Username" value={s.username} hint={`Always "${s.username}".`} />
        </div>
      </Card>
      <Card title="Password" icon={KeyRound} description="Use an API key as the SMTP password. A sending-access key is enough and the safest choice.">
        <div className="flex flex-col gap-3 text-[13px] text-muted">
          <p>The From address must be on a domain verified in this project. Attachments aren&apos;t supported yet, and each message can have up to 50 recipients.</p>
          {superAdmin ? <div><ButtonLink size="sm" href="/keys/">Create an API key</ButtonLink></div> : <p>Ask a project owner for an API key.</p>}
        </div>
      </Card>
    </div>
  );
}
