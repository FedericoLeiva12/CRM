import { useEffect, useMemo, useRef, useState } from 'react';
import { useFetcher, useNavigation, useSearchParams } from '@remix-run/react';
import { RecordCell } from './record-cell';
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ArrowUpRight,
  CircleHelp,
  Database,
  Plus,
  ShieldCheck,
  Trash2,
  Users,
} from 'lucide-react';
import type { CRMRecord, RecordPage, Section } from '../../types/crm';
import { ListToolbar } from './list-toolbar';
import {
  UPDATED_AT,
  hasRefinements,
  listParamsOnly,
  orderedFields,
  parseListState,
  withListState,
  type ListState,
} from './list-query';

interface Props {
  section: Section;
  page: RecordPage;
  canManage: boolean;
  onOpenRecord: (record: CRMRecord | null) => void;
  onAddField: () => void;
  onDeleteRecord: (record: CRMRecord) => void;
}
interface LoadedMore {
  base: RecordPage;
  records: CRMRecord[];
  cursor: string | null;
}

const emptyState: ListState = { search: '', filters: [], sort: '', dir: 'asc' };

export function RecordsView({
  section,
  page,
  canManage,
  onOpenRecord,
  onAddField,
  onDeleteRecord,
}: Props) {
  const [params, setParams] = useSearchParams();
  const state = useMemo(() => parseListState(params), [params]);
  const navigation = useNavigation();
  const fetcher = useFetcher<RecordPage>();
  const [loaded, setLoaded] = useState<LoadedMore | null>(null);
  const requestedFor = useRef<RecordPage | null>(null);

  // "Load more" pages belong to the page they extend; a new query discards them.
  const extra = loaded?.base === page ? loaded : null;
  const records = extra ? [...page.records, ...extra.records] : page.records;
  const nextCursor = extra ? extra.cursor : page.nextCursor;
  const loadError = fetcher.data?.error && requestedFor.current === page ? fetcher.data.error : '';

  useEffect(() => {
    const result = fetcher.data;
    if (!result || result.error || requestedFor.current !== page) return;
    setLoaded((current) => {
      const base = current?.base === page ? current : { base: page, records: [], cursor: null };
      const known = new Set([...page.records, ...base.records].map((record) => record.id));
      const fresh = result.records.filter((record) => !known.has(record.id));
      return { base: page, records: [...base.records, ...fresh], cursor: result.nextCursor };
    });
  }, [fetcher.data, page]);

  function update(next: ListState, options?: { replace?: boolean }) {
    setParams(withListState(params, next), {
      replace: options?.replace,
      preventScrollReset: true,
    });
  }
  function loadMore() {
    if (!nextCursor) return;
    const query = listParamsOnly(params);
    query.set('cursor', nextCursor);
    requestedFor.current = page;
    fetcher.load(`/records/${encodeURIComponent(section.id)}/query?${query}`);
  }
  function toggleSort(field: string) {
    if (state.sort !== field) update({ ...state, sort: field, dir: 'asc' });
    else if (state.dir === 'asc') update({ ...state, dir: 'desc' });
    else update({ ...state, sort: '', dir: 'asc' });
  }

  const columns = orderedFields(section.fields);
  const refined = hasRefinements(state);
  const loading = navigation.state === 'loading';
  const loadingMore = fetcher.state !== 'idle';
  return (
    <>
      <div className="summary">
        <div>
          <span>
            {refined ? 'Matching' : 'Total'} {section.name.toLowerCase()}
          </span>
          <strong>{page.total.toString().padStart(2, '0')}</strong>
        </div>
        <div>
          <span>Showing</span>
          <strong>
            {records.length.toString().padStart(2, '0')}
            <small>of {page.total} records</small>
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
      <ListToolbar
        sectionName={section.name}
        fields={columns}
        state={state}
        records={records}
        canManage={canManage}
        onChange={update}
        onClear={() => update(emptyState)}
        onManageFields={onAddField}
      />
      {page.error && (
        <div role="alert" className="error-banner">
          {page.error}{' '}
          <button type="button" className="clear-filters" onClick={() => update(emptyState)}>
            Clear filters
          </button>
        </div>
      )}
      <div className={`table-wrap ${loading ? 'is-loading' : ''}`} aria-busy={loading}>
        <table>
          <thead>
            <tr>
              {columns.map((field) => (
                <SortableHeader
                  key={field.id}
                  label={field.label}
                  active={state.sort === field.id}
                  dir={state.dir}
                  onSort={() => toggleSort(field.id)}
                />
              ))}
              <SortableHeader
                label="Updated"
                active={state.sort === UPDATED_AT}
                dir={state.dir}
                onSort={() => toggleSort(UPDATED_AT)}
              />
              <th>
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {records.map((record) => (
              <tr key={record.id} onClick={() => onOpenRecord(record)}>
                {columns.map((field) => (
                  <td key={field.id}>
                    <RecordCell field={field} record={record} onOpenRecord={onOpenRecord} />
                  </td>
                ))}
                <td>
                  <span title={record.updated_at}>{record.updated_at.slice(0, 10)}</span>
                </td>
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
        {records.length === 0 && !page.error && (
          <div className="empty">
            <Users size={32} />
            <h2>{refined ? 'No matching records' : 'Your next relationship starts here.'}</h2>
            <p>
              {refined
                ? 'Try another search or adjust your filters.'
                : `Add your first record to ${section.name} and keep the important details together.`}
            </p>
            {refined ? (
              <button className="secondary" onClick={() => update(emptyState)}>
                Clear filters
              </button>
            ) : (
              <button className="primary" onClick={() => onOpenRecord(null)}>
                <Plus size={17} />
                Add a record
              </button>
            )}
          </div>
        )}
      </div>
      <div className="table-footer">
        <span role="status">
          Showing {records.length} of {page.total} records
        </span>
        <span>
          <ShieldCheck size={14} />
          Saved in your workspace
        </span>
      </div>
      {loadError && (
        <div role="alert" className="error-banner">
          {loadError}
        </div>
      )}
      {nextCursor && (
        <div className="load-more">
          <button className="secondary" onClick={loadMore} disabled={loadingMore}>
            {loadingMore ? 'Loading…' : 'Load more'}
          </button>
        </div>
      )}
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

function SortableHeader({
  label,
  active,
  dir,
  onSort,
}: {
  label: string;
  active: boolean;
  dir: 'asc' | 'desc';
  onSort: () => void;
}) {
  let icon = <ArrowUpDown size={13} />;
  if (active) icon = dir === 'asc' ? <ArrowUp size={13} /> : <ArrowDown size={13} />;
  let ariaSort: 'none' | 'ascending' | 'descending' = 'none';
  if (active) ariaSort = dir === 'asc' ? 'ascending' : 'descending';
  return (
    <th aria-sort={ariaSort}>
      <button type="button" className={`sort-button ${active ? 'active' : ''}`} onClick={onSort}>
        {label}
        {icon}
      </button>
    </th>
  );
}
