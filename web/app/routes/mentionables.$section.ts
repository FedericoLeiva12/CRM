import { json, type LoaderFunctionArgs } from '@remix-run/node';
import { api } from '../api.server';
import type { MentionCandidates } from '../types/crm';

export async function loader({ request, params }: LoaderFunctionArgs) {
  if (!params.section) throw new Response('Not found', { status: 404 });
  try {
    return json(await api<MentionCandidates>(request, `/sections/${params.section}/mentionables`));
  } catch (error) {
    if (error instanceof Response) throw error;
    return json(
      { error: error instanceof Error ? error.message : 'People could not be loaded.' },
      { status: 400 },
    );
  }
}
