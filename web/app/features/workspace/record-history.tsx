import { useFetcher } from '@remix-run/react';
import { useEffect } from 'react';
import type { Activity, RecordDetail, RecordLink } from '../../types/crm';

type HistoryPayload = RecordDetail | { error: string };

function isDetail(value: HistoryPayload): value is RecordDetail {
  return 'activities' in value;
}
function activityWhen(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  const date = new Intl.DateTimeFormat('en', {
    timeZone: 'UTC',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }).format(parsed);
  if (parsed.getUTCHours() === 0 && parsed.getUTCMinutes() === 0 && parsed.getUTCSeconds() === 0) {
    return date;
  }
  const time = new Intl.DateTimeFormat('en', {
    timeZone: 'UTC',
    hour: '2-digit',
    minute: '2-digit',
  }).format(parsed);
  return `${date}, ${time} UTC`;
}
function authorLabel(kind: string): string {
  if (kind === 'agent') return 'Agent';
  if (kind === 'user') return 'Team member';
  return kind;
}
function activityMeta(activity: Activity): string {
  const parts = [activity.type.replaceAll('_', ' '), authorLabel(activity.author.kind)];
  if (activity.channel) parts.push(activity.channel);
  if (activity.ref) parts.push(activity.ref);
  return parts.join(' · ');
}
function linkText(link: RecordLink): string {
  const relation = link.direction === 'incoming' ? 'Linked from' : 'Linked to';
  return `${relation} ${link.section_name} · ${link.name || 'Untitled'}`;
}
export function RecordHistory({ sectionID, recordID }: { sectionID: string; recordID: string }) {
  const history = useFetcher<HistoryPayload>();
  const loadHistory = history.load;
  useEffect(() => {
    // `load` is stable. Depending on the fetcher object retriggers this effect
    // on every state change, and a ref that skips the second run loses the
    // request: Strict Mode aborts the first load during its simulated unmount.
    loadHistory(`/records/${encodeURIComponent(sectionID)}/${encodeURIComponent(recordID)}`);
  }, [loadHistory, sectionID, recordID]);
  const payload = history.data;
  return (
    <section className="record-history" aria-label="History">
      <h2>History</h2>
      {!payload && <p className="history-status">Loading history…</p>}
      {payload && !isDetail(payload) && (
        <p className="history-status" role="alert">
          {payload.error}
        </p>
      )}
      {payload && isDetail(payload) && payload.links.length > 0 && (
        <ul className="record-links">
          {payload.links.map((link) => (
            <li key={`${link.direction}-${link.section_id}-${link.record_id}`}>{linkText(link)}</li>
          ))}
        </ul>
      )}
      {payload && isDetail(payload) && payload.activities.length === 0 && (
        <p className="history-status">No activity yet.</p>
      )}
      {payload && isDetail(payload) && payload.activities.length > 0 && (
        <ol className="timeline">
          {payload.activities.map((activity) => (
            <li key={activity.id}>
              <time dateTime={activity.date}>{activityWhen(activity.date)}</time>
              <div>
                <p>{activity.summary}</p>
                <p className="timeline-meta">{activityMeta(activity)}</p>
              </div>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
