import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { ArrowDown, ArrowDownUp, ArrowUp, Filter, Plus, Search, Settings2, X } from 'lucide-react';
import type { CRMRecord, Field } from '../../types/crm';
import {
  UPDATED_AT,
  describeFilter,
  hasListState,
  joinList,
  operatorsFor,
  serializeFilter,
  type ListFilter,
  type ListState,
  type OperatorChoice,
} from './list-query';

interface ToolbarProps {
  sectionName: string;
  fields: Field[];
  state: ListState;
  records: CRMRecord[];
  canManage: boolean;
  onChange: (state: ListState, options?: { replace?: boolean }) => void;
  onClear: () => void;
  onManageFields: () => void;
}

const SEARCH_DELAY_MS = 300;

export function ListToolbar({
  sectionName,
  fields,
  state,
  records,
  canManage,
  onChange,
  onClear,
  onManageFields,
}: ToolbarProps) {
  const [builderOpen, setBuilderOpen] = useState(false);
  const [searchText, setSearchText] = useState(state.search);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const latest = useRef(state);
  latest.current = state;

  // Follow the URL for back/forward and shared links, but never while the user is typing.
  useEffect(() => {
    if (timer.current === null)
      setSearchText((current) => (current.trim() === state.search ? current : state.search));
  }, [state.search]);
  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  function submitSearch(text: string) {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    if (text.trim() !== latest.current.search)
      onChange({ ...latest.current, search: text }, { replace: true });
  }
  function typeSearch(text: string) {
    setSearchText(text);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => submitSearch(text), SEARCH_DELAY_MS);
  }
  function sortBy(value: string) {
    if (!value) onChange({ ...state, sort: '', dir: 'asc' });
    else onChange({ ...state, sort: value, dir: state.sort === value ? state.dir : 'asc' });
  }
  function removeFilter(index: number) {
    onChange({ ...state, filters: state.filters.filter((_, position) => position !== index) });
  }

  return (
    <div className="list-toolbar">
      <div className="list-toolbar-row">
        <form
          className="search"
          role="search"
          onSubmit={(event) => {
            event.preventDefault();
            submitSearch(searchText);
          }}
        >
          <Search size={17} aria-hidden="true" />
          <input
            type="search"
            aria-label={`Search ${sectionName}`}
            placeholder={`Search ${sectionName.toLowerCase()}…`}
            value={searchText}
            maxLength={200}
            onChange={(event) => typeSearch(event.target.value)}
          />
        </form>
        <div className="list-toolbar-actions">
          <button
            type="button"
            className={`secondary ${builderOpen ? 'active' : ''}`}
            aria-expanded={builderOpen}
            aria-controls="filter-builder"
            onClick={() => setBuilderOpen((open) => !open)}
          >
            <Filter size={16} />
            <span>Filter</span>
            {state.filters.length > 0 && <b className="count-badge">{state.filters.length}</b>}
          </button>
          <label className="sort-select">
            <ArrowDownUp size={15} aria-hidden="true" />
            <span className="sr-only">Sort by</span>
            <select value={state.sort} onChange={(event) => sortBy(event.target.value)}>
              <option value="">Recently updated</option>
              {fields.map((field) => (
                <option key={field.id} value={field.id}>
                  {field.label}
                </option>
              ))}
              <option value={UPDATED_AT}>Last updated</option>
            </select>
          </label>
          {state.sort && (
            <button
              type="button"
              className="secondary direction"
              onClick={() => onChange({ ...state, dir: state.dir === 'asc' ? 'desc' : 'asc' })}
              aria-label={`Sorted ${state.dir === 'asc' ? 'ascending' : 'descending'}. Reverse the order`}
            >
              {state.dir === 'asc' ? <ArrowUp size={15} /> : <ArrowDown size={15} />}
              <span>{state.dir === 'asc' ? 'Ascending' : 'Descending'}</span>
            </button>
          )}
          {canManage && (
            <button type="button" className="secondary" onClick={onManageFields}>
              <Settings2 size={16} />
              <span>Fields</span>
            </button>
          )}
        </div>
      </div>
      {builderOpen && (
        <FilterBuilder
          fields={fields}
          records={records}
          onCancel={() => setBuilderOpen(false)}
          onAdd={(added) => {
            onChange({ ...state, filters: [...state.filters, ...added] });
            setBuilderOpen(false);
          }}
        />
      )}
      {hasListState(state) && (
        <div className="filter-chips" aria-label="Active filters">
          {state.filters.map((filter, index) => {
            const parts = describeFilter(filter, fields);
            return (
              <span className="chip" key={`${serializeFilter(filter)}-${index}`}>
                <b>{parts.field}</b> {parts.operator} {parts.value && <i>{parts.value}</i>}
                <button
                  type="button"
                  aria-label={`Remove filter ${parts.field} ${parts.operator} ${parts.value}`}
                  onClick={() => removeFilter(index)}
                >
                  <X size={13} />
                </button>
              </span>
            );
          })}
          <button type="button" className="clear-filters" onClick={onClear}>
            Clear filters
          </button>
        </div>
      )}
    </div>
  );
}

function suggestionsFor(fieldId: string, records: CRMRecord[]): string[] {
  const seen = new Set<string>();
  for (const record of records) {
    const value = record.data[fieldId];
    if (typeof value === 'string' && value && seen.size < 30) seen.add(value);
  }
  return [...seen].sort((first, second) => first.localeCompare(second));
}

interface BuilderProps {
  fields: Field[];
  records: CRMRecord[];
  onAdd: (filters: ListFilter[]) => void;
  onCancel: () => void;
}

function FilterBuilder({ fields, records, onAdd, onCancel }: BuilderProps) {
  const [fieldId, setFieldId] = useState(fields[0]?.id || '');
  const field = fields.find((candidate) => candidate.id === fieldId) || fields[0];
  const operators = useMemo(() => (field ? operatorsFor(field.type) : []), [field]);
  const [choice, setChoice] = useState<OperatorChoice>(operators[0]?.op || 'eq');
  const [value, setValue] = useState('');
  const [upper, setUpper] = useState('');
  const option = operators.find((candidate) => candidate.op === choice) || operators[0];
  const suggestions = useMemo(
    () => (field && option?.input === 'list' ? suggestionsFor(field.id, records) : []),
    [field, option, records],
  );

  if (!field || !option) return null;

  function selectField(identifier: string) {
    const next = fields.find((candidate) => candidate.id === identifier);
    setFieldId(identifier);
    setChoice(next ? operatorsFor(next.type)[0].op : 'eq');
    setValue(next?.type === 'boolean' ? 'true' : '');
    setUpper('');
  }
  function selectOperator(next: OperatorChoice) {
    setChoice(next);
    const input = operators.find((candidate) => candidate.op === next)?.input;
    setValue(input === 'boolean' ? 'true' : '');
    setUpper('');
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    if (choice === 'between')
      onAdd([
        { field: field.id, op: 'gte', value },
        { field: field.id, op: 'lte', value: upper },
      ]);
    else if (choice === 'in') {
      const values = value
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);
      if (values.length === 0) return;
      onAdd([{ field: field.id, op: 'in', value: joinList(values) }]);
    } else onAdd([{ field: field.id, op: choice, value: option.input === 'none' ? '' : value }]);
  }

  const inputType = field.type === 'number' || field.type === 'date' ? field.type : 'text';
  return (
    <form id="filter-builder" className="filter-builder" onSubmit={submit}>
      <label>
        Field
        <select value={field.id} onChange={(event) => selectField(event.target.value)}>
          {fields.map((candidate) => (
            <option key={candidate.id} value={candidate.id}>
              {candidate.label}
            </option>
          ))}
        </select>
      </label>
      <label>
        Condition
        <select
          value={option.op}
          onChange={(event) => selectOperator(event.target.value as OperatorChoice)}
        >
          {operators.map((candidate) => (
            <option key={candidate.op} value={candidate.op}>
              {candidate.label}
            </option>
          ))}
        </select>
      </label>
      {option.input === 'boolean' && (
        <label>
          Value
          <select value={value} onChange={(event) => setValue(event.target.value)}>
            <option value="true">Yes</option>
            <option value="false">No</option>
          </select>
        </label>
      )}
      {(option.input === 'text' || option.input === 'number' || option.input === 'date') && (
        <label>
          Value
          <input
            type={inputType}
            step={inputType === 'number' ? 'any' : undefined}
            value={value}
            required
            autoFocus
            onChange={(event) => setValue(event.target.value)}
          />
        </label>
      )}
      {option.input === 'range' && (
        <>
          <label>
            From
            <input
              type={inputType}
              step={inputType === 'number' ? 'any' : undefined}
              value={value}
              required
              autoFocus
              onChange={(event) => setValue(event.target.value)}
            />
          </label>
          <label>
            To
            <input
              type={inputType}
              step={inputType === 'number' ? 'any' : undefined}
              value={upper}
              required
              onChange={(event) => setUpper(event.target.value)}
            />
          </label>
        </>
      )}
      {option.input === 'list' && (
        <label className="wide">
          Values
          <input
            type="text"
            value={value}
            required
            autoFocus
            list="filter-suggestions"
            placeholder="Separate values with commas"
            onChange={(event) => setValue(event.target.value)}
          />
          <datalist id="filter-suggestions">
            {suggestions.map((suggestion) => (
              <option key={suggestion} value={suggestion} />
            ))}
          </datalist>
        </label>
      )}
      <div className="filter-builder-actions">
        <button type="submit" className="primary">
          <Plus size={16} />
          Add filter
        </button>
        <button type="button" className="secondary" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}
