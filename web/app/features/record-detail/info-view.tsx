import { useFetcher } from '@remix-run/react';
import { Pencil } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { Field, CRMRecord } from '../../types/crm';
import { RecordFields } from '../workspace/editor-fields';
import { orderedFields } from '../workspace/list-query';
import type { workspaceAction } from '../workspace/workspace.server';
import type { ItemViewProps } from './types';

function displayValue(field: Field, value: CRMRecord['data'][string] | undefined) {
  if (value === undefined || value === null || value === '') return '—';
  if (field.type === 'boolean') return value ? 'Yes' : 'No';
  if (field.type === 'number' && typeof value === 'number')
    return new Intl.NumberFormat('en').format(value);
  return String(value);
}

export function InfoView({ section, record, onPendingChangesChange }: ItemViewProps) {
  const fetcher = useFetcher<typeof workspaceAction>();
  const [editing, setEditing] = useState(false);
  const [dirty, setDirty] = useState(false);
  const handled = useRef(fetcher.data);
  const busy = fetcher.state !== 'idle';
  useEffect(() => {
    if (busy || !fetcher.data || handled.current === fetcher.data) return;
    handled.current = fetcher.data;
    if (fetcher.data.ok) {
      setEditing(false);
      setDirty(false);
      onPendingChangesChange(false);
    }
  }, [busy, fetcher.data, onPendingChangesChange]);
  function cancelEdit() {
    setEditing(false);
    setDirty(false);
    onPendingChangesChange(false);
  }
  const fields = orderedFields(section.fields);
  return (
    <section className="item-info" aria-label="Item information">
      <div className="item-view-heading">
        <h2>Information</h2>
        {!editing && (
          <button type="button" className="secondary" onClick={() => setEditing(true)}>
            <Pencil size={16} />
            Edit
          </button>
        )}
      </div>
      {fetcher.data?.ok && !editing && (
        <p role="status" className="success-message">
          Changes saved.
        </p>
      )}
      {editing ? (
        <fetcher.Form
          method="post"
          className="item-edit-form"
          onChange={() => {
            setDirty(true);
            onPendingChangesChange(true);
          }}
        >
          <input type="hidden" name="intent" value="save" />
          <input type="hidden" name="section" value={section.id} />
          <fieldset disabled={busy}>
            <RecordFields fields={fields} record={record} />
          </fieldset>
          {fetcher.data?.error && (
            <p role="alert" className="error-banner">
              {fetcher.data.error}
            </p>
          )}
          <div className="item-edit-actions">
            <button type="button" className="secondary" disabled={busy} onClick={cancelEdit}>
              Cancel
            </button>
            <button type="submit" className="primary" disabled={busy || !dirty}>
              {busy ? 'Saving…' : 'Save changes'}
            </button>
          </div>
        </fetcher.Form>
      ) : (
        <dl className="item-fields">
          {fields.map((field) => (
            <div key={field.id} className={field.id === 'notes' ? 'item-field-wide' : undefined}>
              <dt>{field.label}</dt>
              <dd className={field.type === 'number' ? 'numeric' : undefined}>
                {displayValue(field, record.data[field.id])}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}
