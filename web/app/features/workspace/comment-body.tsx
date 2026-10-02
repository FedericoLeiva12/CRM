import type { ReactNode } from 'react';
import type { Principal } from '../../types/crm';

// Same word-boundary rule as the server: an email address or a path is not a mention.
const wordBefore = /[A-Za-z0-9_@/.+-]/;
const tokens = /`[^`\n]+`|\*\*[^*\n]+\*\*|@[A-Za-z0-9][A-Za-z0-9_-]{0,31}/g;

export function MentionChip({ principal, label }: { principal: Principal; label: string }) {
  const title = principal.name ? `${principal.name} · ${principal.kind}` : principal.kind;
  return (
    <span className={`mention-chip ${principal.kind}`} title={title}>
      {label}
    </span>
  );
}

// Renders markdown-ish plain text: `code`, **bold**, line breaks, and @mentions as chips.
// Only handles stored as structured mentions become chips; any other @text stays text.
export function CommentBody({ text, mentions }: { text: string; mentions: Principal[] }) {
  const known = new Map(mentions.map((principal) => [principal.handle.toLowerCase(), principal]));
  const nodes: ReactNode[] = [];
  let cursor = 0;
  for (const match of text.matchAll(tokens)) {
    const start = match.index;
    const value = match[0];
    let node: ReactNode = null;
    let end = start + value.length;
    if (value.startsWith('`')) {
      node = <code>{value.slice(1, -1)}</code>;
    } else if (value.startsWith('**')) {
      node = <strong>{value.slice(2, -2)}</strong>;
    } else if (start === 0 || !wordBefore.test(text[start - 1])) {
      const trimmed = value.replace(/[-_]+$/, '');
      const principal = known.get(trimmed.slice(1).toLowerCase());
      if (principal) {
        node = <MentionChip principal={principal} label={trimmed} />;
        end = start + trimmed.length;
      }
    }
    if (!node) continue;
    if (start > cursor) nodes.push(text.slice(cursor, start));
    nodes.push(<span key={start}>{node}</span>);
    cursor = end;
  }
  if (cursor < text.length) nodes.push(text.slice(cursor));
  return <p className="comment-body">{nodes}</p>;
}
