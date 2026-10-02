import type { Field, FieldType } from '../../types/crm';

/**
 * List view state lives in the URL so a view can be shared and survives a reload:
 *   q=<free text>   f=<field>:<op>:<value> (repeatable)   sort=<field|updated_at>   dir=desc
 * Everything here is derived from the section schema; no section or field ids are known.
 */
export type FilterOp =
  'contains' | 'eq' | 'neq' | 'in' | 'gt' | 'gte' | 'lt' | 'lte' | 'is_empty' | 'not_empty';
export type OperatorChoice = FilterOp | 'between';
export type ValueInput = 'text' | 'number' | 'date' | 'boolean' | 'list' | 'range' | 'none';

export interface ListFilter {
  field: string;
  op: FilterOp;
  value: string;
}
export type SortDirection = 'asc' | 'desc';
export interface ListState {
  search: string;
  filters: ListFilter[];
  sort: string;
  dir: SortDirection;
}
export interface OperatorOption {
  op: OperatorChoice;
  label: string;
  input: ValueInput;
}

export const UPDATED_AT = 'updated_at';
export const PAGE_SIZE = 50;
const MAX_FILTERS = 20;
const LIST_PARAMS = ['q', 'f', 'sort', 'dir'];

const filterOps: ReadonlySet<string> = new Set<FilterOp>([
  'contains',
  'eq',
  'neq',
  'in',
  'gt',
  'gte',
  'lt',
  'lte',
  'is_empty',
  'not_empty',
]);
const emptiness: OperatorOption[] = [
  { op: 'is_empty', label: 'is empty', input: 'none' },
  { op: 'not_empty', label: 'is not empty', input: 'none' },
];

const operatorsByType: Record<FieldType, OperatorOption[]> = {
  text: [
    { op: 'contains', label: 'contains', input: 'text' },
    { op: 'eq', label: 'is', input: 'text' },
    { op: 'neq', label: 'is not', input: 'text' },
    { op: 'in', label: 'is any of', input: 'list' },
    ...emptiness,
  ],
  email: [
    { op: 'contains', label: 'contains', input: 'text' },
    { op: 'eq', label: 'is', input: 'text' },
    { op: 'neq', label: 'is not', input: 'text' },
    { op: 'in', label: 'is any of', input: 'list' },
    ...emptiness,
  ],
  number: [
    { op: 'eq', label: 'equals', input: 'number' },
    { op: 'neq', label: 'does not equal', input: 'number' },
    { op: 'gt', label: 'greater than', input: 'number' },
    { op: 'gte', label: 'at least', input: 'number' },
    { op: 'lt', label: 'less than', input: 'number' },
    { op: 'lte', label: 'at most', input: 'number' },
    { op: 'between', label: 'is between', input: 'range' },
    { op: 'in', label: 'is any of', input: 'list' },
    ...emptiness,
  ],
  date: [
    { op: 'eq', label: 'is on', input: 'date' },
    { op: 'neq', label: 'is not on', input: 'date' },
    { op: 'gt', label: 'is after', input: 'date' },
    { op: 'gte', label: 'is on or after', input: 'date' },
    { op: 'lt', label: 'is before', input: 'date' },
    { op: 'lte', label: 'is on or before', input: 'date' },
    { op: 'between', label: 'is between', input: 'range' },
    ...emptiness,
  ],
  boolean: [{ op: 'eq', label: 'is', input: 'boolean' }, ...emptiness],
};

export function operatorsFor(type: FieldType): OperatorOption[] {
  return operatorsByType[type];
}

export function joinList(values: string[]): string {
  return values.map((value) => value.replace(/[\\,]/g, (match) => `\\${match}`)).join(',');
}
export function splitList(value: string): string[] {
  const values: string[] = [];
  let current = '';
  for (let index = 0; index < value.length; index += 1) {
    const character = value[index];
    if (character === '\\' && index + 1 < value.length) {
      index += 1;
      current += value[index];
    } else if (character === ',') {
      values.push(current);
      current = '';
    } else current += character;
  }
  values.push(current);
  return values;
}

function parseFilter(raw: string): ListFilter | null {
  const first = raw.indexOf(':');
  const second = raw.indexOf(':', first + 1);
  const field = first > 0 ? raw.slice(0, first) : '';
  const op = second > first ? raw.slice(first + 1, second) : raw.slice(first + 1);
  if (!field || !filterOps.has(op)) return null;
  return { field, op: op as FilterOp, value: second > first ? raw.slice(second + 1) : '' };
}
export function serializeFilter(filter: ListFilter): string {
  return `${filter.field}:${filter.op}:${filter.value}`;
}

export function parseListState(params: URLSearchParams): ListState {
  const filters = params
    .getAll('f')
    .map(parseFilter)
    .filter((filter): filter is ListFilter => filter !== null)
    .slice(0, MAX_FILTERS);
  const sort = params.get('sort') || '';
  return {
    search: params.get('q') || '',
    filters,
    sort,
    dir: sort && params.get('dir') === 'desc' ? 'desc' : 'asc',
  };
}

/** Writes the list state into a copy of `base`, leaving unrelated parameters (section, view) alone. */
export function withListState(base: URLSearchParams, state: ListState): URLSearchParams {
  const next = new URLSearchParams(base);
  for (const name of LIST_PARAMS) next.delete(name);
  if (state.search.trim()) next.set('q', state.search.trim());
  for (const filter of state.filters) next.append('f', serializeFilter(filter));
  if (state.sort) {
    next.set('sort', state.sort);
    if (state.dir === 'desc') next.set('dir', 'desc');
  }
  return next;
}
export function hasListState(state: ListState): boolean {
  return state.search.trim() !== '' || state.filters.length > 0 || state.sort !== '';
}
export function hasRefinements(state: ListState): boolean {
  return state.search.trim() !== '' || state.filters.length > 0;
}
export function listParamsOnly(params: URLSearchParams): URLSearchParams {
  const only = new URLSearchParams();
  for (const name of LIST_PARAMS) for (const value of params.getAll(name)) only.append(name, value);
  return only;
}

function typedValue(type: FieldType | undefined, raw: string): string | number | boolean {
  if (type === 'number') {
    const number = raw.trim() === '' ? Number.NaN : Number(raw);
    return Number.isFinite(number) ? number : raw;
  }
  if (type === 'boolean') return raw === 'true';
  return raw;
}

export interface ApiQuery {
  filters: { field: string; op: FilterOp; value?: unknown }[];
  search: string;
  sort?: { field: string; direction: SortDirection };
  limit: number;
  cursor?: string;
}

/** Converts URL strings to the typed body of POST /sections/{section}/records/query. */
export function toApiQuery(
  state: ListState,
  fields: Field[],
  options: { limit: number; cursor?: string },
): ApiQuery {
  const types = new Map(fields.map((field) => [field.id, field.type]));
  const query: ApiQuery = {
    filters: state.filters.map((filter) => {
      const type = types.get(filter.field);
      if (filter.op === 'is_empty' || filter.op === 'not_empty')
        return { field: filter.field, op: filter.op };
      if (filter.op === 'in')
        return {
          field: filter.field,
          op: filter.op,
          value: splitList(filter.value).map((value) => typedValue(type, value)),
        };
      return { field: filter.field, op: filter.op, value: typedValue(type, filter.value) };
    }),
    search: state.search.trim(),
    limit: options.limit,
  };
  if (state.sort) query.sort = { field: state.sort, direction: state.dir };
  if (options.cursor) query.cursor = options.cursor;
  return query;
}

export function operatorLabel(type: FieldType | undefined, op: FilterOp): string {
  if (!type) return op;
  return operatorsByType[type].find((option) => option.op === op)?.label || op;
}

/** Human-readable parts of a filter, for the chips under the toolbar. */
export function describeFilter(filter: ListFilter, fields: Field[]) {
  const field = fields.find((candidate) => candidate.id === filter.field);
  let value = '';
  if (filter.op === 'in') value = splitList(filter.value).join(', ');
  else if (field?.type === 'boolean' && filter.op === 'eq')
    value = filter.value === 'true' ? 'Yes' : 'No';
  else if (filter.op !== 'is_empty' && filter.op !== 'not_empty') value = filter.value;
  return {
    field: field?.label || filter.field,
    operator: operatorLabel(field?.type, filter.op),
    value,
  };
}

/** Every section is created with a name field, so it leads; the rest keep schema order. */
export function orderedFields(fields: Field[]): Field[] {
  return [...fields].sort(
    (first, second) => Number(second.id === 'name') - Number(first.id === 'name'),
  );
}
