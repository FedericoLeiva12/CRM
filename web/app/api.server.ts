import { redirect } from '@remix-run/node';

export class APIError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'APIError';
  }
}

export async function api<T = unknown>(request: Request, path: string, options: RequestInit = {}) {
  const response = await fetch(`${process.env.API_URL || 'http://localhost:8080'}/api${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      Cookie: request.headers.get('Cookie') || '',
      Origin: process.env.APP_ORIGIN || 'http://localhost:3000',
      ...options.headers,
    },
  });
  if (response.status === 401) throw redirect('/login');
  const data: unknown = await response.json();
  if (!response.ok)
    throw new APIError(
      response.status,
      typeof data === 'object' && data !== null && 'error' in data
        ? String(data.error)
        : 'Request failed',
    );
  return data as T;
}
export function requireOrigin(request: Request) {
  if (
    request.method !== 'GET' &&
    request.headers.get('Origin') !== (process.env.APP_ORIGIN || new URL(request.url).origin)
  )
    throw new Response('Invalid origin', { status: 403 });
}
