import { json, redirect, type LoaderFunctionArgs, type ActionFunctionArgs } from '@remix-run/node';
import { api, requireOrigin } from '../../api.server';
import type {
  Agent,
  CreatedAgent,
  CreatedInvite,
  CRMRecord,
  Invite,
  Section,
  WorkspaceUser,
  WorkspaceView,
} from '../../types/crm';

function textValue(form: FormData, name: string): string {
  const value = form.get(name);
  if (value instanceof File) throw new Error(`Invalid ${name}`);
  return value || '';
}
function checkedValue(form: FormData, name: string): boolean {
  return form.get(name) === 'on';
}

export async function workspaceLoader({ request }: LoaderFunctionArgs) {
  const currentUser = await api<WorkspaceUser>(request, '/me');
  const sections = await api<Section[]>(request, '/sections');
  const searchParams = new URL(request.url).searchParams;
  const section =
    sections.find((section) => section.id === searchParams.get('section')) || sections[0];
  const requestedView = searchParams.get('view');
  const view: WorkspaceView =
    requestedView === 'fields' || requestedView === 'agents' || requestedView === 'team'
      ? requestedView
      : 'records';
  if (view !== 'records' && currentUser.role !== 'admin') throw redirect('/');
  const [records, agents, users, invites] = await Promise.all([
    view === 'records' && section
      ? api<CRMRecord[]>(request, `/sections/${section.id}/records`)
      : Promise.resolve([]),
    view === 'agents' ? api<Agent[]>(request, '/agents') : Promise.resolve([]),
    view === 'team' ? api<WorkspaceUser[]>(request, '/users') : Promise.resolve([]),
    view === 'team' ? api<Invite[]>(request, '/invites') : Promise.resolve([]),
  ]);
  return json({ sections, section, view, records, agents, users, invites, currentUser });
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
      default:
        throw new Error('Unknown action');
    }
    return json({
      ok: true,
      intent,
      token: createdToken,
      inviteLink,
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
        error: error instanceof Error ? error.message : 'Unable to save',
      },
      { status: 400 },
    );
  }
}
