import { useFetcher } from '@remix-run/react';
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import type {
  Activity,
  ActivityAuthor,
  MentionCandidates,
  Principal,
  RecordDetail,
  RecordLink,
  Viewer,
} from '../../types/crm';
import { CommentBody } from './comment-body';
import { CommentComposer, initials } from './comment-composer';

type HistoryPayload = RecordDetail | { error: string };
type PeoplePayload = MentionCandidates | { error: string };
interface ActionPayload {
  ok: boolean;
  intent: string;
  unresolved: string[];
  error: string;
}

function isDetail(value: HistoryPayload): value is RecordDetail {
  return 'activities' in value;
}
function hasPeople(value: PeoplePayload): value is MentionCandidates {
  return 'users' in value;
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
function authorLabel(author: ActivityAuthor): string {
  if (author.name) return author.name;
  return author.kind === 'agent' ? 'Agent' : 'Team member';
}
function activityMeta(activity: Activity): string {
  const parts = [activity.type.replaceAll('_', ' '), authorLabel(activity.author)];
  if (activity.channel) parts.push(activity.channel);
  if (activity.ref) parts.push(activity.ref);
  return parts.join(' · ');
}
function linkText(link: RecordLink): string {
  const relation = link.direction === 'incoming' ? 'Linked from' : 'Linked to';
  return `${relation} ${link.section_name} · ${link.name || 'Untitled'}`;
}
function unresolvedNotice(unresolved: string[]): string {
  if (unresolved.length === 0) return '';
  return `Saved. ${unresolved.join(', ')} could not be notified: unknown, or no access to this section.`;
}

// One fetcher per form keeps simultaneous edits, replies, and deletes independent.
function useCommentAction(url: string, onSaved: () => void) {
  const fetcher = useFetcher<ActionPayload>();
  const handled = useRef<ActionPayload | null>(null);
  const [notice, setNotice] = useState('');
  const saved = useRef(onSaved);
  useEffect(() => {
    saved.current = onSaved;
  });
  useEffect(() => {
    const result = fetcher.data;
    if (fetcher.state !== 'idle' || !result || handled.current === result) return;
    handled.current = result;
    if (result.ok) {
      setNotice(unresolvedNotice(result.unresolved));
      saved.current();
    }
  }, [fetcher.state, fetcher.data]);
  const failure = fetcher.data && !fetcher.data.ok ? fetcher.data.error : '';
  return {
    busy: fetcher.state !== 'idle',
    error: failure,
    notice,
    send(values: Record<string, string>) {
      setNotice('');
      fetcher.submit(values, { method: 'post', action: url });
    },
  };
}

interface EntryProps {
  activity: Activity;
  replies: Activity[];
  viewer: Viewer;
  people: Principal[];
  url: string;
  onChanged: () => void;
}

function Avatar({ author }: { author: ActivityAuthor }) {
  return (
    <span className={`avatar ${author.kind}`} aria-hidden="true">
      {initials(author.name || author.handle || author.kind)}
    </span>
  );
}

function CommentMeta({ activity }: { activity: Activity }) {
  return (
    <div className="comment-head">
      <Avatar author={activity.author} />
      <b>{authorLabel(activity.author)}</b>
      {activity.author.handle && <span className="muted-handle">@{activity.author.handle}</span>}
      {activity.author.kind === 'agent' && <span className="kind-tag agent">Agent</span>}
      {activity.edited_at && (
        <span className="edited" title={activityWhen(activity.edited_at)}>
          edited
        </span>
      )}
    </div>
  );
}

function CommentView({
  activity,
  replies,
  viewer,
  people,
  url,
  onChanged,
  nested,
}: EntryProps & { nested?: boolean }) {
  const [mode, setMode] = useState<'view' | 'edit' | 'reply' | 'delete'>('view');
  const edit = useCommentAction(url, () => {
    setMode('view');
    onChanged();
  });
  const reply = useCommentAction(url, () => {
    setMode('view');
    onChanged();
  });
  const remove = useCommentAction(url, onChanged);
  const mine = activity.author.kind === 'user' && activity.author.id === viewer.id;
  const canEdit = mine && !activity.deleted;
  const canDelete = mine || viewer.role === 'admin';
  const liveReplies = replies.filter((item) => !item.deleted).length;

  let content: ReactNode;
  if (mode === 'edit') {
    content = (
      <CommentComposer
        label="Edit comment"
        placeholder="Edit your comment"
        submitLabel="Save"
        people={people}
        busy={edit.busy}
        error={edit.error}
        initialText={activity.summary}
        autoFocus
        onCancel={() => setMode('view')}
        onSubmit={(text) => edit.send({ intent: 'comment-edit', id: activity.id, body: text })}
      />
    );
  } else if (activity.deleted) {
    content = <p className="comment-body deleted">This comment was deleted.</p>;
  } else {
    content = <CommentBody text={activity.summary} mentions={activity.mentions ?? []} />;
  }

  return (
    <article
      className={nested ? 'comment reply' : 'comment'}
      aria-label={`Comment by ${authorLabel(activity.author)}`}
    >
      <CommentMeta activity={activity} />
      {content}
      {mode !== 'edit' && !activity.deleted && (
        <div className="comment-actions">
          {!nested && (
            <button type="button" className="text-button" onClick={() => setMode('reply')}>
              Reply
            </button>
          )}
          {canEdit && (
            <button type="button" className="text-button" onClick={() => setMode('edit')}>
              Edit
            </button>
          )}
          {canDelete && mode !== 'delete' && (
            <button
              type="button"
              className="text-button destructive"
              onClick={() => setMode('delete')}
            >
              Delete
            </button>
          )}
        </div>
      )}
      {mode === 'delete' && (
        <div className="confirm-row" role="alertdialog" aria-label="Confirm delete">
          <span>
            Delete this comment?
            {!nested && liveReplies > 0 && ' Its replies stay in the thread.'}
          </span>
          <button
            type="button"
            className="danger compact"
            disabled={remove.busy}
            onClick={() => remove.send({ intent: 'comment-delete', id: activity.id })}
          >
            {remove.busy ? 'Deleting…' : 'Delete'}
          </button>
          <button type="button" className="secondary compact" onClick={() => setMode('view')}>
            Keep
          </button>
          {remove.error && (
            <span className="composer-error" role="alert">
              {remove.error}
            </span>
          )}
        </div>
      )}
      {!nested && (replies.length > 0 || mode === 'reply') && (
        <div className="replies">
          {replies.map((item) => (
            <CommentView
              key={item.id}
              activity={item}
              replies={[]}
              viewer={viewer}
              people={people}
              url={url}
              onChanged={onChanged}
              nested
            />
          ))}
          {mode === 'reply' && (
            <CommentComposer
              label={`Reply to ${authorLabel(activity.author)}`}
              placeholder="Write a reply"
              submitLabel="Reply"
              people={people}
              busy={reply.busy}
              error={reply.error}
              autoFocus
              onCancel={() => setMode('view')}
              onSubmit={(text) =>
                reply.send({ intent: 'comment', parent_id: activity.id, body: text })
              }
            />
          )}
        </div>
      )}
    </article>
  );
}

export function RecordHistory({
  sectionID,
  recordID,
  viewer,
}: {
  sectionID: string;
  recordID: string;
  viewer: Viewer;
}) {
  const history = useFetcher<HistoryPayload>();
  const directory = useFetcher<PeoplePayload>();
  const loadHistory = history.load;
  const loadPeople = directory.load;
  const url = `/records/${encodeURIComponent(sectionID)}/${encodeURIComponent(recordID)}`;
  useEffect(() => {
    // `load` is stable. Depending on the fetcher object retriggers this effect
    // on every state change, and a ref that skips the second run loses the
    // request: Strict Mode aborts the first load during its simulated unmount.
    loadHistory(url);
  }, [loadHistory, url]);
  useEffect(() => {
    loadPeople(`/mentionables/${encodeURIComponent(sectionID)}`);
  }, [loadPeople, sectionID]);
  const reload = useCallback(() => loadHistory(url), [loadHistory, url]);
  const [composerKey, setComposerKey] = useState(0);
  const create = useCommentAction(url, () => {
    setComposerKey((key) => key + 1);
    reload();
  });

  const people = useMemo<Principal[]>(() => {
    const payload = directory.data;
    if (!payload || !hasPeople(payload)) return [];
    return [...payload.users, ...payload.agents].filter(
      (person) => !(person.kind === 'user' && person.id === viewer.id),
    );
  }, [directory.data, viewer.id]);

  const payload = history.data;
  const detail = payload && isDetail(payload) ? payload : null;
  const { roots, repliesByParent } = useMemo(() => {
    const grouped = new Map<string, Activity[]>();
    const top: Activity[] = [];
    for (const activity of detail?.activities ?? []) {
      if (activity.parent_id) {
        grouped.set(activity.parent_id, [...(grouped.get(activity.parent_id) ?? []), activity]);
      } else top.push(activity);
    }
    for (const list of grouped.values()) {
      list.sort((a, b) => a.created_at.localeCompare(b.created_at));
    }
    return { roots: top, repliesByParent: grouped };
  }, [detail]);

  return (
    <section className="record-history" aria-label="History">
      <h2>History</h2>
      <CommentComposer
        key={composerKey}
        label="Add a comment"
        placeholder="Add a comment. Type @ to mention someone."
        submitLabel="Comment"
        people={people}
        busy={create.busy}
        error={create.error}
        notice={create.notice}
        onSubmit={(text) => create.send({ intent: 'comment', body: text })}
      />
      {!payload && <p className="history-status">Loading history…</p>}
      {payload && !isDetail(payload) && (
        <p className="history-status" role="alert">
          {payload.error}
        </p>
      )}
      {detail && detail.links.length > 0 && (
        <ul className="record-links">
          {detail.links.map((link) => (
            <li key={`${link.direction}-${link.section_id}-${link.record_id}`}>{linkText(link)}</li>
          ))}
        </ul>
      )}
      {detail && roots.length === 0 && <p className="history-status">No activity yet.</p>}
      {detail && roots.length > 0 && (
        <ol className="timeline">
          {roots.map((activity) => (
            <li
              key={activity.id}
              className={activity.type === 'comment' ? 'is-comment' : undefined}
            >
              <time dateTime={activity.date}>{activityWhen(activity.date)}</time>
              {activity.type === 'comment' ? (
                <CommentView
                  activity={activity}
                  replies={repliesByParent.get(activity.id) ?? []}
                  viewer={viewer}
                  people={people}
                  url={url}
                  onChanged={reload}
                />
              ) : (
                <div>
                  <p>{activity.summary}</p>
                  <p className="timeline-meta">{activityMeta(activity)}</p>
                </div>
              )}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
