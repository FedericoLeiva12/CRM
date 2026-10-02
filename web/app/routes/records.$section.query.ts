import { json, type LoaderFunctionArgs } from '@remix-run/node';
import { api } from '../api.server';
import { loadRecordPage } from '../features/workspace/workspace.server';
import type { Section } from '../types/crm';

// Serves "load more": the same URL parameters as the list view plus an opaque cursor.
export async function loader({ request, params }: LoaderFunctionArgs) {
  const sections = await api<Section[]>(request, '/sections');
  const section = sections.find((candidate) => candidate.id === params.section);
  if (!section) throw new Response('Not found', { status: 404 });
  const searchParams = new URL(request.url).searchParams;
  return json(
    await loadRecordPage(request, section, searchParams, searchParams.get('cursor') || ''),
  );
}
