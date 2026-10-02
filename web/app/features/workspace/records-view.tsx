import { useState } from 'react';
import { RecordCell } from './record-cell';
import {
  ArrowUpRight,
  CircleHelp,
  Database,
  Plus,
  Search,
  Settings2,
  ShieldCheck,
  Trash2,
  Users,
} from 'lucide-react';
import type { CRMRecord, Section } from '../../types/crm';
interface Props {
  section: Section;
  records: CRMRecord[];
  canManage: boolean;
  onOpenRecord: (record: CRMRecord | null) => void;
  onAddField: () => void;
  onDeleteRecord: (record: CRMRecord) => void;
}
export function RecordsView({
  section,
  records,
  canManage,
  onOpenRecord,
  onAddField,
  onDeleteRecord,
}: Props) {
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState('all');
  const statuses = Array.from(
    new Set(records.map((record) => String(record.data.status || '')).filter(Boolean)),
  );
  const filtered = records.filter(
    (record) =>
      Object.values(record.data).some((value) =>
        String(value).toLowerCase().includes(search.toLowerCase()),
      ) &&
      (filter === 'all' || record.data.status === filter),
  );
  const recordsWithEmailCount = records.filter((record) => record.data.email).length;
  const fields = section?.fields || [];
  const columns = [...fields]
    .filter((field) => field.id !== 'notes')
    .sort((firstField, secondField) => {
      const order = ['name', 'company', 'email', 'status', 'value'];
      return (
        (order.includes(firstField.id) ? order.indexOf(firstField.id) : 99) -
        (order.includes(secondField.id) ? order.indexOf(secondField.id) : 99)
      );
    })
    .slice(0, 6);
  return (
    <>
      <div className="summary">
        <div>
          <span>Total {section?.name.toLowerCase()}</span>
          <strong>{records.length.toString().padStart(2, '0')}</strong>
        </div>
        <div>
          <span>With email</span>
          <strong>
            {recordsWithEmailCount.toString().padStart(2, '0')}
            <small>of {records.length} records</small>
          </strong>
        </div>
        <div className="summary-note">
          <Database size={22} />
          <p>
            A place for every detail.
            <br />
            {canManage ? (
              <button onClick={() => onAddField()}>
                Customize your fields <ArrowUpRight size={14} />
              </button>
            ) : (
              <span>Your team keeps these details together.</span>
            )}
          </p>
        </div>
      </div>
      <div className="table-toolbar">
        <div className="filter-tabs">
          <button className={filter === 'all' ? 'selected' : ''} onClick={() => setFilter('all')}>
            All {section?.name.toLowerCase()} <span>{records.length}</span>
          </button>
          {statuses.slice(0, 3).map((status) => (
            <button
              key={status}
              className={filter === status ? 'selected' : ''}
              onClick={() => setFilter(status)}
            >
              {status}
            </button>
          ))}
        </div>
        <div className="table-controls">
          <label className="search">
            <Search size={17} />
            <input
              aria-label="Search records"
              placeholder="Search relationships…"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </label>
          {canManage && (
            <button className="secondary" onClick={() => onAddField()}>
              <Settings2 size={16} />
              <span>Fields</span>
            </button>
          )}
        </div>
      </div>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              {columns.map((field) => (
                <th key={field.id}>{field.label}</th>
              ))}
              <th>
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((record) => (
              <tr key={record.id} onClick={() => onOpenRecord(record)}>
                {columns.map((field) => (
                  <td key={field.id}>
                    <RecordCell field={field} record={record} onOpenRecord={onOpenRecord} />
                  </td>
                ))}
                <td>
                  <button
                    className="icon-button"
                    aria-label={`Delete ${record.data.name}`}
                    onClick={(event) => {
                      event.stopPropagation();
                      onDeleteRecord(record);
                    }}
                  >
                    <Trash2 size={15} />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {filtered.length === 0 && (
          <div className="empty">
            <Users size={32} />
            <h2>
              {search || filter !== 'all'
                ? 'No matching relationships'
                : 'Your next relationship starts here.'}
            </h2>
            <p>
              {search || filter !== 'all'
                ? 'Try another search or filter.'
                : `Add your first ${
                    section?.id === 'prospects' ? 'prospect' : 'client'
                  } and keep the important details together.`}
            </p>
            {!search && filter === 'all' && (
              <button className="primary" onClick={() => onOpenRecord(null)}>
                <Plus size={17} />
                Add a record
              </button>
            )}
          </div>
        )}
      </div>
      <div className="table-footer">
        <span>
          {filtered.length} of {records.length} records
          {records.length === 500 ? ' · Latest 500' : ''}
        </span>
        <span>
          <ShieldCheck size={14} />
          Saved in your workspace
        </span>
      </div>
      {canManage && (
        <div className="bottom-note">
          <CircleHelp size={17} />
          <span>Make it yours. Add custom fields for the details your team tracks.</span>
          <button onClick={() => onAddField()}>
            Manage fields <ArrowUpRight size={14} />
          </button>
        </div>
      )}
    </>
  );
}
