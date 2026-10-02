import { useEffect, useId, useMemo, useRef, useState } from 'react';
import type { Principal } from '../../types/crm';

const maxLength = 4000;
const maxSuggestions = 6;
const mentionTrigger = /(?:^|[^A-Za-z0-9_@/.+-])@([A-Za-z0-9_-]{0,32})$/;

export function initials(name: string) {
  const parts = name.split(/[\s._-]+/).filter(Boolean);
  const letters = parts.length > 1 ? `${parts[0][0]}${parts[1][0]}` : name.slice(0, 2);
  return letters.toUpperCase() || '?';
}

function suggestionsFor(people: Principal[], query: string): Principal[] {
  const needle = query.toLowerCase();
  const rank = (person: Principal) => {
    const handle = person.handle.toLowerCase();
    const name = (person.name || '').toLowerCase();
    if (handle.startsWith(needle)) return 0;
    if (name.split(/\s+/).some((word) => word.startsWith(needle))) return 1;
    if (handle.includes(needle) || name.includes(needle)) return 2;
    return -1;
  };
  return people
    .map((person) => ({ person, score: rank(person) }))
    .filter((item) => item.score >= 0)
    .sort((a, b) => a.score - b.score)
    .slice(0, maxSuggestions)
    .map((item) => item.person);
}

interface Props {
  label: string;
  placeholder: string;
  submitLabel: string;
  people: Principal[];
  busy: boolean;
  error?: string;
  notice?: string;
  initialText?: string;
  autoFocus?: boolean;
  onSubmit: (text: string) => void;
  onCancel?: () => void;
}

export function CommentComposer({
  label,
  placeholder,
  submitLabel,
  people,
  busy,
  error,
  notice,
  initialText = '',
  autoFocus,
  onSubmit,
  onCancel,
}: Props) {
  const [text, setText] = useState(initialText);
  const [caret, setCaret] = useState(initialText.length);
  const [active, setActive] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const field = useRef<HTMLTextAreaElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const pendingCaret = useRef<number | null>(null);
  const id = useId();
  const listId = `${id}-people`;

  const trigger = mentionTrigger.exec(text.slice(0, caret));
  const query = trigger ? trigger[1] : null;
  const matches = useMemo(
    () => (query === null ? [] : suggestionsFor(people, query)),
    [people, query],
  );
  const open = matches.length > 0 && !dismissed;
  const current = Math.min(active, Math.max(matches.length - 1, 0));

  useEffect(() => {
    if (pendingCaret.current !== null && field.current) {
      field.current.setSelectionRange(pendingCaret.current, pendingCaret.current);
      pendingCaret.current = null;
    }
  }, [text]);
  useEffect(() => {
    if (open) list.current?.scrollIntoView({ block: 'nearest' });
  }, [open]);
  useEffect(() => {
    if (autoFocus) field.current?.focus();
  }, [autoFocus]);

  function choose(person: Principal) {
    if (query === null) return;
    const start = caret - query.length - 1;
    const insert = `@${person.handle} `;
    setText(text.slice(0, start) + insert + text.slice(caret));
    pendingCaret.current = start + insert.length;
    setCaret(start + insert.length);
    setActive(0);
    field.current?.focus();
  }
  function submit() {
    const trimmed = text.trim();
    if (trimmed && !busy) onSubmit(trimmed);
  }

  return (
    <div className="composer">
      <label className="sr-only" htmlFor={`${id}-field`}>
        {label}
      </label>
      <div className="composer-field">
        <textarea
          id={`${id}-field`}
          ref={field}
          value={text}
          maxLength={maxLength}
          placeholder={placeholder}
          rows={3}
          role="combobox"
          aria-expanded={open}
          aria-autocomplete="list"
          aria-controls={listId}
          aria-activedescendant={open ? `${id}-option-${current}` : undefined}
          onChange={(event) => {
            setText(event.target.value);
            setCaret(event.target.selectionStart);
            setDismissed(false);
            setActive(0);
          }}
          onSelect={(event) => setCaret(event.currentTarget.selectionStart)}
          onKeyDown={(event) => {
            if (open && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
              event.preventDefault();
              const step = event.key === 'ArrowDown' ? 1 : matches.length - 1;
              setActive((current + step) % matches.length);
            } else if (open && (event.key === 'Enter' || event.key === 'Tab')) {
              event.preventDefault();
              choose(matches[current]);
            } else if (open && event.key === 'Escape') {
              event.preventDefault();
              setDismissed(true);
            } else if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
              event.preventDefault();
              submit();
            }
          }}
        />
        {open && (
          <ul
            className="mention-list"
            id={listId}
            role="listbox"
            aria-label="People to mention"
            ref={list}
          >
            {matches.map((person, index) => (
              <li
                key={`${person.kind}-${person.id}`}
                id={`${id}-option-${index}`}
                role="option"
                aria-selected={index === current}
                className={index === current ? 'selected' : ''}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(person)}
                onMouseEnter={() => setActive(index)}
              >
                <span className={`avatar ${person.kind}`} aria-hidden="true">
                  {initials(person.name || person.handle)}
                </span>
                <span className="mention-name">{person.name || person.handle}</span>
                <span className="mention-handle">@{person.handle}</span>
                <span className={`kind-tag ${person.kind}`}>
                  {person.kind === 'agent' ? 'Agent' : 'Team'}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
      <div className="composer-bar">
        <small className="composer-hint">
          Type @ to mention a teammate or agent. Ctrl+Enter sends.
        </small>
        <div className="composer-buttons">
          {onCancel && (
            <button type="button" className="secondary compact" onClick={onCancel}>
              Cancel
            </button>
          )}
          <button
            type="button"
            className="primary compact"
            disabled={busy || text.trim() === ''}
            onClick={submit}
          >
            {busy ? 'Saving…' : submitLabel}
          </button>
        </div>
      </div>
      {error && (
        <p className="composer-error" role="alert">
          {error}
        </p>
      )}
      {notice && !error && (
        <p className="composer-notice" role="status">
          {notice}
        </p>
      )}
    </div>
  );
}
