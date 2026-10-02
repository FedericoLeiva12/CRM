import { json, type ActionFunctionArgs, type LoaderFunctionArgs } from '@remix-run/node';
import { api, requireOrigin } from '../api.server';
import type { RecordDetail } from '../types/crm';

export async function loader({ request, params }: LoaderFunctionArgs) {
  if (!params.section || !params.id) throw new Response('Not found', { status: 404 });
  try {
    return json(
      await api<RecordDetail>(request, `/sections/${params.section}/records/${params.id}`),
    );
  } catch (error) {
    if (error instanceof Response) throw error;
    return json(
      { error: error instanceof Error ? error.message : 'History could not be loaded.' },
      { status: 400 },
    );
  }
}

function field(form: FormData, name: string): string {
  const value = form.get(name);
  return typeof value === 'string' ? value : '';
}

interface SavedComment {
  unresolved_mentions?: string[];
}

export async function action({ request, params }: ActionFunctionArgs) {
  requireOrigin(request);
  if (!params.section || !params.id) throw new Response('Not found', { status: 404 });
  const form = await request.formData();
  const intent = field(form, 'intent');
  try {
    let saved: SavedComment = {};
    if (intent === 'comment') {
      saved = await api<SavedComment>(
        request,
        `/sections/${params.section}/records/${params.id}/comments`,
        {
          method: 'POST',
          body: JSON.stringify({
            body: field(form, 'body'),
            parent_id: field(form, 'parent_id'),
          }),
        },
      );
    } else if (intent === 'comment-edit') {
      saved = await api<SavedComment>(
        request,
        `/comments/${encodeURIComponent(field(form, 'id'))}`,
        {
          method: 'PUT',
          body: JSON.stringify({ body: field(form, 'body') }),
        },
      );
    } else if (intent === 'comment-delete') {
      await api(request, `/comments/${encodeURIComponent(field(form, 'id'))}`, {
        method: 'DELETE',
      });
    } else {
      throw new Error('Unknown action');
    }
    return json({ ok: true, intent, unresolved: saved.unresolved_mentions ?? [], error: '' });
  } catch (error) {
    if (error instanceof Response) throw error;
    return json(
      {
        ok: false,
        intent,
        unresolved: [] as string[],
        error: error instanceof Error ? error.message : 'Unable to save the comment.',
      },
      { status: 400 },
    );
  }
}
