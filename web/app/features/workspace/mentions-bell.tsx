import { Link, useFetcher } from '@remix-run/react';
import { Bell, Bot } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { MentionNotification } from '../../types/crm';

type FeedPayload = { mentions: MentionNotification[]; unread_count: number } | { error: string };

const refreshMs = 45_000;

function hasMentions(value: FeedPayload): value is Extract<FeedPayload, { mentions: unknown }> {
  return 'mentions' in value;
}
function timeAgo(value: string): string {
  const seconds = Math.max(0, Math.round((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 60) return 'just now';
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.round(hours / 24);
  if (days < 30) return `${days} d ago`;
  return new Date(value).toLocaleDateString('en', { month: 'short', day: 'numeric' });
}
function authorName(mention: MentionNotification): string {
  return mention.author.name || (mention.author.kind === 'agent' ? 'An agent' : 'A teammate');
}

export function MentionsBell() {
  const feed = useFetcher<FeedPayload>();
  const marker = useFetcher<{ ok: boolean }>();
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const loadFeed = feed.load;
  const marked = marker.data;
  const markerState = marker.state;

  useEffect(() => {
    loadFeed('/mentions');
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') loadFeed('/mentions');
    }, refreshMs);
    return () => window.clearInterval(timer);
  }, [loadFeed]);
  useEffect(() => {
    if (open) loadFeed('/mentions');
  }, [open, loadFeed]);
  useEffect(() => {
    if (markerState === 'idle' && marked) loadFeed('/mentions');
  }, [markerState, marked, loadFeed]);
  useEffect(() => {
    if (!open) return;
    function onPointer(event: MouseEvent) {
      if (container.current && !container.current.contains(event.target as Node)) setOpen(false);
    }
    function onKey(event: KeyboardEvent) {
      if (event.key !== 'Escape') return;
      setOpen(false);
      trigger.current?.focus();
    }
    document.addEventListener('mousedown', onPointer);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onPointer);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  const payload = feed.data;
  const loaded = payload && hasMentions(payload) ? payload : null;
  const unread = loaded?.unread_count ?? 0;

  function markRead(values: Record<string, string>) {
    marker.submit(values, { method: 'post', action: '/mentions' });
  }

  return (
    <div className="bell" ref={container}>
      <button
        ref={trigger}
        type="button"
        className="bell-button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={unread > 0 ? `Mentions, ${unread} unread` : 'Mentions'}
        onClick={() => setOpen((value) => !value)}
      >
        <Bell size={19} />
        {unread > 0 && (
          <span className="bell-badge" aria-hidden="true">
            {unread > 9 ? '9+' : unread}
          </span>
        )}
      </button>
      {open && (
        <div className="bell-panel" role="dialog" aria-label="Mentions">
          <div className="bell-heading">
            <h2>Mentions</h2>
            <button
              type="button"
              className="text-button"
              disabled={unread === 0 || markerState !== 'idle'}
              onClick={() => markRead({ all: 'true' })}
            >
              Mark all as read
            </button>
          </div>
          {!payload && <p className="bell-empty">Loading mentions…</p>}
          {payload && !loaded && (
            <p className="bell-empty" role="alert">
              {'error' in payload ? payload.error : 'Mentions could not be loaded.'}
            </p>
          )}
          {loaded && loaded.mentions.length === 0 && (
            <p className="bell-empty">
              Nothing yet. When someone @mentions you on a record, it shows up here.
            </p>
          )}
          {loaded && loaded.mentions.length > 0 && (
            <ul className="bell-list">
              {loaded.mentions.map((mention) => (
                <li key={mention.id} className={mention.read_at ? 'read' : 'unread'}>
                  <Link
                    to={`/?section=${encodeURIComponent(mention.section_id)}&record=${encodeURIComponent(mention.record_id)}`}
                    onClick={() => {
                      if (!mention.read_at) markRead({ id: mention.id });
                      setOpen(false);
                    }}
                  >
                    <span
                      className="bell-dot"
                      aria-label={mention.read_at ? 'Read' : 'Unread'}
                      role="img"
                    />
                    <span className="bell-text">
                      <span className="bell-line">
                        <b>{authorName(mention)}</b>
                        {mention.author.kind === 'agent' && <Bot size={13} aria-label="Agent" />}
                        <span> mentioned you</span>
                      </span>
                      <span className="bell-where">
                        {mention.section_name} · {mention.record_name || 'Untitled record'}
                      </span>
                      <span className="bell-body">{mention.body}</span>
                      <time dateTime={mention.created_at}>{timeAgo(mention.created_at)}</time>
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
