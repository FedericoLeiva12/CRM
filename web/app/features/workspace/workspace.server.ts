import { json, redirect, type LoaderFunctionArgs, type ActionFunctionArgs } from '@remix-run/node';
import { api, APIError, requireOrigin } from '../../api.server';
import type {
  Agent,
  CreatedAgent,
  CreatedInvite,
  Invite,
  RecordPage,
  RecordDetail,
  ItemViewDefinition,
  Section,
  WebhookDelivery,
  WebhookEndpoint,
  WorkspaceUser,
  WorkspaceView,
} from '../../types/crm';
import { settingsGroups } from './settings';
import { PAGE_SIZE, parseListState, toApiQuery } from './list-query';

interface QueryResponse {
  records: RecordPage['records'];
  next_cursor: string | null;
  total: number;
}

function textValue(form: FormData, name: string): string {
  const value = form.get(name);
  if (value instanceof File) throw new Error(`Invalid ${name}`);
  return value || '';
}
function checkedValue(form: FormData, name: string): boolean {
  return form.get(name) === 'on';
}
const webhookEventIds = [
  'record.created',
  'record.updated',
  'record.deleted',
  'section.created',
  'field.created',
  'timeline.entry_created',
  'comment.mentioned',
];
function webhookExcludedActors(form: FormData) {
  const excluded: { kind: 'user' | 'agent'; id: string }[] = [];
  for (const [key, value] of form.entries()) {
    if (value !== 'on' || typeof key !== 'string' || !key.startsWith('exclude:')) continue;
    const parts = key.slice('exclude:'.length).split(':');
    if (parts.length !== 2) continue;
    const kind = parts[0];
    if (kind !== 'user' && kind !== 'agent') continue;
    excluded.push({ kind, id: parts[1] });
  }
  return excluded;
}
function webhookBody(form: FormData) {
  return {
    url: textValue(form, 'url'),
    description: textValue(form, 'description'),
    event_types: webhookEventIds.filter((eventType) => form.get(`event:${eventType}`) === 'on'),
    section_id: textValue(form, 'section_id'),
    enabled: checkedValue(form, 'enabled'),
    excluded_actors: webhookExcludedActors(form),
    signing_secret: textValue(form, 'signing_secret'),
    clear_signing_secret: checkedValue(form, 'clear_signing_secret'),
    custom_header_name: textValue(form, 'custom_header_name'),
    custom_header_value: textValue(form, 'custom_header_value'),
    clear_custom_header: checkedValue(form, 'clear_custom_header'),
  };
}

// Item links (including mentions) open the same addressable detail screen.
async function openedRecord(request: Request, sectionID: string, params: URLSearchParams) {
  const recordID = params.get('record');
  if (!recordID) return null;
  try {
    return await api<RecordDetail>(
      request,
      `/sections/${sectionID}/records/${encodeURIComponent(recordID)}`,
    );
  } catch (error) {
    if (error instanceof Response) throw error;
    if (error instanceof APIError)
      throw new Response(error.message, {
        status: error.status,
        statusText: error.status === 404 ? 'Item not found' : 'Unable to load item',
      });
    throw error;
  }
}

/**
 * One page of the shared filter/sort/search query. A rejected query (for example a
 * hand-edited URL) becomes an inline error so the toolbar can still be used to fix it.
 */
export async function loadRecordPage(
  request: Request,
  section: Section,
  params: URLSearchParams,
  cursor = '',
): Promise<RecordPage> {
  const body = toApiQuery(parseListState(params), section.fields, { limit: PAGE_SIZE, cursor });
  try {
    const result = await api<QueryResponse>(request, `/sections/${section.id}/records/query`, {
      method: 'POST',
      body: JSON.stringify(body),
    });
    return {
      records: result.records,
      total: result.total,
      nextCursor: result.next_cursor,
      error: null,
    };
  } catch (error) {
    if (error instanceof Response || error instanceof TypeError || error instanceof SyntaxError)
      throw error;
    return {
      records: [],
      total: 0,
      nextCursor: null,
      error: error instanceof Error ? error.message : 'Unable to load records',
    };
  }
}

export async function workspaceLoader({ request }: LoaderFunctionArgs) {
  const currentUser = await api<WorkspaceUser>(request, '/me');
  const sections = await api<Section[]>(request, '/sections');
  const searchParams = new URL(request.url).searchParams;
  const section =
    sections.find((section) => section.id === searchParams.get('section')) || sections[0];
  if (
    searchParams.has('record') &&
    !sections.some((section) => section.id === searchParams.get('section'))
  )
    throw new Response('Section not found', { status: 404, statusText: 'Section not found' });
  const requestedView = searchParams.get('view');
  if (requestedView === 'settings') {
    throw redirect(currentUser.role === 'admin' ? '/?view=fields' : '/?view=security');
  }
  const setting = settingsGroups
    .flatMap((group) => group.items)
    .find((item) => item.view === requestedView);
  const view: WorkspaceView = setting?.view || 'records';
  if (setting?.adminOnly && currentUser.role !== 'admin') throw redirect('/?view=security');
  const focusRecord =
    view === 'records' && section ? await openedRecord(request, section.id, searchParams) : null;
  const webhooks = view === 'webhooks' ? await api<WebhookEndpoint[]>(request, '/webhooks') : [];
  const requestedEndpoint = searchParams.get('endpoint') || '';
  const selectedWebhook = webhooks.find((endpoint) => endpoint.id === requestedEndpoint);
  const [page, agents, users, invites, deliveries] = await Promise.all([
    view === 'records' && section && !focusRecord
      ? loadRecordPage(request, section, searchParams)
      : Promise.resolve<RecordPage>({ records: [], total: 0, nextCursor: null, error: null }),
    view === 'agents' || view === 'webhooks'
      ? api<Agent[]>(request, '/agents')
      : Promise.resolve([]),
    view === 'team' || view === 'webhooks'
      ? api<WorkspaceUser[]>(request, '/users')
      : Promise.resolve([]),
    view === 'team' ? api<Invite[]>(request, '/invites') : Promise.resolve([]),
    selectedWebhook
      ? api<WebhookDelivery[]>(request, `/webhooks/${selectedWebhook.id}/deliveries`)
      : Promise.resolve([]),
  ]);
  return json({
    sections,
    section,
    view,
    page,
    agents,
    users,
    invites,
    webhooks,
    deliveries,
    selectedWebhookId: selectedWebhook?.id || '',
    focusRecord,
    currentUser,
    itemViewCatalog:
      view === 'fields' || focusRecord
        ? await api<ItemViewDefinition[]>(request, '/item-view-types')
        : [],
    sidebarCollapsed: /(?:^|;\s*)sira_sidebar=collapsed(?:;|$)/.test(
      request.headers.get('Cookie') || '',
    ),
  });
}

async function saveRelationship(request: Request, form: FormData, sectionID: string) {
  const sections = await api<Section[]>(request, '/sections');
  const section = sections.find((section) => section.id === sectionID);
  if (!section) throw new Error('Section not found');
  const values: Record<string, string | number | boolean> = {};
  // Read the current registry rather than trusting field names submitted by the browser.
  for (const field of section.fields) {
    if (field.type === 'boolean') {
      values[field.id] = checkedValue(form, `field:${field.id}`);
      continue;
    }
    const value = textValue(form, `field:${field.id}`);
    if (value === '') continue;
    if (field.type === 'number') {
      const number = Number(value);
      if (!Number.isFinite(number)) throw new Error(`${field.label} must be a finite number`);
      values[field.id] = number;
    } else values[field.id] = value;
  }
  const recordID = textValue(form, 'id');
  await api(request, `/sections/${sectionID}/records${recordID ? `/${recordID}` : ''}`, {
    method: recordID ? 'PUT' : 'POST',
    body: JSON.stringify({ data: values }),
  });
}
async function savePermissions(request: Request, form: FormData) {
  const sections = await api<Section[]>(request, '/sections');
  await api(request, `/agents/${textValue(form, 'id')}/permissions`, {
    method: 'PUT',
    body: JSON.stringify({
      manage_schema: checkedValue(form, 'manage_schema'),
      permissions: sections.map((section) => ({
        section_id: section.id,
        read: checkedValue(form, `${section.id}:read`),
        write: checkedValue(form, `${section.id}:write`),
        delete: checkedValue(form, `${section.id}:delete`),
      })),
    }),
  });
}
async function signOut(request: Request) {
  const response = await fetch(`${process.env.API_URL || 'http://localhost:8080'}/api/logout`, {
    method: 'POST',
    headers: {
      Cookie: request.headers.get('Cookie') || '',
      Origin: process.env.APP_ORIGIN || 'http://localhost:3000',
    },
  });
  if (!response.ok && response.status !== 401)
    throw new Error('Unable to sign out. Please try again.');
  return redirect('/login', {
    headers: {
      'Set-Cookie':
        response.headers.get('Set-Cookie') ||
        'sira_session=; Path=/; HttpOnly; SameSite=Strict; Max-Age=0',
    },
  });
}
export async function workspaceAction({ request }: ActionFunctionArgs) {
  requireOrigin(request);
  const form = await request.formData();
  const intent = textValue(form, 'intent');
  const sectionID = textValue(form, 'section');
  try {
    let createdToken: string | undefined;
    let inviteLink: string | undefined;
    let endpointId: string | undefined;
    switch (intent) {
      case 'logout':
        return await signOut(request);
      case 'password':
        await api(request, '/password', {
          method: 'POST',
          body: JSON.stringify({
            current: textValue(form, 'current'),
            password: textValue(form, 'password'),
          }),
        });
        return redirect('/login', {
          headers: { 'Set-Cookie': 'sira_session=; Path=/; HttpOnly; SameSite=Strict; Max-Age=0' },
        });
      case 'save':
        await saveRelationship(request, form, sectionID);
        break;
      case 'delete':
        await api(request, `/sections/${sectionID}/records/${textValue(form, 'id')}`, {
          method: 'DELETE',
        });
        break;
      case 'item-views': {
        const views: unknown = JSON.parse(textValue(form, 'views'));
        await api(request, `/sections/${sectionID}/views`, {
          method: 'PUT',
          body: JSON.stringify({ views }),
        });
        break;
      }
      case 'field':
        await api(request, `/sections/${sectionID}/fields`, {
          method: 'POST',
          body: JSON.stringify({
            id: textValue(form, 'key'),
            label: textValue(form, 'label'),
            type: textValue(form, 'type'),
            required: checkedValue(form, 'required'),
          }),
        });
        break;
      case 'section':
        await api(request, '/sections', {
          method: 'POST',
          body: JSON.stringify({ id: textValue(form, 'key'), name: textValue(form, 'name') }),
        });
        break;
      case 'agent': {
        const agent = await api<CreatedAgent>(request, '/agents', {
          method: 'POST',
          body: JSON.stringify({ name: textValue(form, 'name') }),
        });
        createdToken = agent.token;
        break;
      }
      case 'revoke':
        await api(request, `/agents/${textValue(form, 'id')}`, { method: 'DELETE' });
        break;
      case 'permissions':
        await savePermissions(request, form);
        break;
      case 'invite': {
        const invite = await api<CreatedInvite>(request, '/invites', {
          method: 'POST',
          body: JSON.stringify({
            email: textValue(form, 'email'),
            role: textValue(form, 'role'),
          }),
        });
        inviteLink = invite.link;
        break;
      }
      case 'revoke-invite':
        await api(request, `/invites/${textValue(form, 'id')}`, { method: 'DELETE' });
        break;
      case 'role':
        await api(request, `/users/${textValue(form, 'id')}/role`, {
          method: 'PUT',
          body: JSON.stringify({ role: textValue(form, 'role') }),
        });
        break;
      case 'remove-user':
        await api(request, `/users/${textValue(form, 'id')}`, { method: 'DELETE' });
        break;
      case 'webhook-create':
        await api(request, '/webhooks', {
          method: 'POST',
          body: JSON.stringify(webhookBody(form)),
        });
        break;
      case 'webhook-update':
        await api(request, `/webhooks/${textValue(form, 'id')}`, {
          method: 'PUT',
          body: JSON.stringify(webhookBody(form)),
        });
        break;
      case 'webhook-delete':
        await api(request, `/webhooks/${textValue(form, 'id')}`, { method: 'DELETE' });
        break;
      case 'webhook-enable':
        await api(request, `/webhooks/${textValue(form, 'id')}/enable`, { method: 'POST' });
        break;
      case 'webhook-disable':
        await api(request, `/webhooks/${textValue(form, 'id')}/disable`, { method: 'POST' });
        break;
      case 'webhook-test':
        endpointId = textValue(form, 'id');
        await api(request, `/webhooks/${endpointId}/test`, { method: 'POST' });
        break;
      default:
        throw new Error('Unknown action');
    }
    return json({
      ok: true,
      intent,
      token: createdToken,
      inviteLink,
      endpointId,
      error: undefined as string | undefined,
    });
  } catch (error) {
    if (error instanceof Response) throw error;
    return json(
      {
        ok: false,
        intent,
        token: undefined as string | undefined,
        inviteLink: undefined as string | undefined,
        endpointId: undefined as string | undefined,
        error: error instanceof Error ? error.message : 'Unable to save',
      },
      { status: 400 },
    );
  }
}
