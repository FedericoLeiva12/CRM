import { json, type ActionFunctionArgs, type LoaderFunctionArgs } from '@remix-run/node';
import { api, requireOrigin } from '../api.server';
import type { MentionNotification } from '../types/crm';

interface MentionList {
  mentions: MentionNotification[];
  unread_count: number;
}

export async function loader({ request }: LoaderFunctionArgs) {
  try {
    return json(await api<MentionList>(request, '/mentions?limit=30'));
  } catch (error) {
    if (error instanceof Response) throw error;
    return json(
      { error: error instanceof Error ? error.message : 'Mentions could not be loaded.' },
      { status: 400 },
    );
  }
}

export async function action({ request }: ActionFunctionArgs) {
  requireOrigin(request);
  const form = await request.formData();
  const ids = form.getAll('id').filter((value): value is string => typeof value === 'string');
  try {
    await api(request, '/mentions/read', {
      method: 'POST',
      body: JSON.stringify({ ids, all: form.get('all') === 'true' }),
    });
    return json({ ok: true, error: '' });
  } catch (error) {
    if (error instanceof Response) throw error;
    return json(
      { ok: false, error: error instanceof Error ? error.message : 'Unable to update mentions.' },
      { status: 400 },
    );
  }
}
