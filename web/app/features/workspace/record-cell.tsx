import type { CRMRecord, Field } from '../../types/crm';
interface Props {
  field: Field;
  record: CRMRecord;
  onOpenRecord: (record: CRMRecord) => void;
}
export function RecordCell({ field, record, onOpenRecord }: Props) {
  const value = record.data[field.id];
  if (field.id === 'name')
    return (
      <button
        className="record-name"
        onClick={(event) => {
          event.stopPropagation();
          onOpenRecord(record);
        }}
      >
        <span className={`avatar color-${record.id.charCodeAt(0) % 4}`}>
          {String(record.data.name || '?')
            .slice(0, 2)
            .toUpperCase()}
        </span>
        <b>{String(record.data.name || 'Unnamed')}</b>
      </button>
    );
  if (field.id === 'status')
    return (
      <span className="record-status">
        <span
          className={`status-dot ${String(value).toLowerCase() === 'active' ? '' : 'neutral'}`}
        />
        {String(value || '—')}
      </span>
    );
  if (value === undefined || value === null || value === '') return <span>—</span>;
  if (field.type === 'boolean') return <span>{value ? 'Yes' : 'No'}</span>;
  if (field.type === 'number')
    return (
      <span className="numeric">{typeof value === 'number' ? value.toLocaleString() : '—'}</span>
    );
  return <span title={String(value)}>{String(value)}</span>;
}
