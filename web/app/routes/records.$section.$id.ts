import { json, type LoaderFunctionArgs } from '@remix-run/node';
import { api } from '../api.server';
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
