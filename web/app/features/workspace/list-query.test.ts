import assert from 'node:assert/strict';
import test from 'node:test';
import { joinList, parseListState, splitList, toApiQuery, withListState } from './list-query.ts';
import type { Field } from '../../types/crm.ts';

const fields: Field[] = [
  { id: 'name', label: 'Name', type: 'text', required: true },
  { id: 'amount', label: 'Amount', type: 'number', required: false },
  { id: 'won', label: 'Won', type: 'boolean', required: false },
];

test('parseListState reads search, filters, and sort from the URL', () => {
  const params = new URLSearchParams('q=acme&f=status:in:Lead,Active&sort=amount&dir=desc');
  const state = parseListState(params);
  assert.equal(state.search, 'acme');
  assert.equal(state.filters.length, 1);
  assert.equal(state.filters[0].op, 'in');
  assert.equal(state.sort, 'amount');
  assert.equal(state.dir, 'desc');
});

test('withListState round-trips list parameters without touching section or view', () => {
  const base = new URLSearchParams('section=clients&view=records&q=hello');
  const next = withListState(base, {
    search: 'hello',
    filters: [{ field: 'status', op: 'eq', value: 'Lead' }],
    sort: 'name',
    dir: 'asc',
  });
  assert.equal(next.get('section'), 'clients');
  assert.equal(next.get('view'), 'records');
  assert.equal(next.get('q'), 'hello');
  assert.equal(next.getAll('f')[0], 'status:eq:Lead');
  assert.equal(next.get('sort'), 'name');
  assert.equal(next.get('dir'), null);
});

test('toApiQuery maps URL filters to the records query API body', () => {
  const body = toApiQuery(
    {
      search: 'two words',
      filters: [
        { field: 'amount', op: 'gte', value: '100' },
        { field: 'won', op: 'eq', value: 'true' },
        { field: 'name', op: 'in', value: joinList(['a', 'b']) },
      ],
      sort: 'name',
      dir: 'desc',
    },
    fields,
    { limit: 50 },
  );
  assert.equal(body.search, 'two words');
  assert.equal(body.limit, 50);
  assert.deepEqual(body.sort, { field: 'name', direction: 'desc' });
  assert.equal(body.filters[0].value, 100);
  assert.equal(body.filters[1].value, true);
  assert.deepEqual(body.filters[2].value, ['a', 'b']);
});

test('splitList respects escaped commas', () => {
  assert.deepEqual(splitList('a\\,b,c'), ['a,b', 'c']);
});
