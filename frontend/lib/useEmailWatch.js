import { useEffect, useRef } from 'react';
import { api } from './api';
import { plural } from './format';
import { useSession } from './session';
import { useToast } from './toast';

export function useEmailWatch(id, onReport, restartKey = 0) {
  const { refreshMe } = useSession();
  const toast = useToast();
  const onReportRef = useRef(onReport);
  onReportRef.current = onReport;

  useEffect(() => {
    if (!id) return undefined;
    let stopped = false;
    let timer = null;
    let after = 0;
    let lastStatus = null;

    async function drainMessages() {
      for (;;) {
        const { messages } = await api('GET', `/api/emails/${id}/messages?after=${after}&limit=200`);
        for (const m of messages) {
          console.log(`[forke] rendered message #${m.id}`, m.payload);
          after = m.id;
        }
        if (messages.length < 200) return;
      }
    }

    async function tick() {
      try {
        const report = await api('GET', `/api/emails/${id}/report`);
        await drainMessages();
        if (stopped) return;
        onReportRef.current(report);
        if (lastStatus === 'sending' && report.status === 'done') {
          toast(`Email sent to ${plural(report.counts.sent, 'contact')}.`, 'success', 6000);
          refreshMe();
        }
        if (lastStatus === 'sending' && report.status === 'paused') {
          toast(`Sending paused: ${String(report.pause_reason).replace(/_/g, ' ')}.`, 'warn', 6000);
          refreshMe();
        }
        lastStatus = report.status;
        if (report.status === 'sending' || report.status === 'paused') {
          timer = setTimeout(tick, report.status === 'sending' ? 1000 : 3000);
        }
      } catch {
        if (!stopped) timer = setTimeout(tick, 3000);
      }
    }

    tick();
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [id, restartKey, refreshMe, toast]);
}
