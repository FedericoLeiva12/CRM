import {
  Form,
  isRouteErrorResponse,
  useActionData,
  useLoaderData,
  useNavigation,
  useRouteError,
} from '@remix-run/react';
import { json, redirect, type ActionFunctionArgs, type LoaderFunctionArgs } from '@remix-run/node';
import { requireOrigin } from '../api.server';
import { roleLabel } from '../features/workspace/presentation';

function apiOrigin() {
  return process.env.API_URL || 'http://localhost:8080';
}
function errorMessage(data: unknown, fallback: string) {
  if (typeof data === 'object' && data !== null && 'error' in data) return String(data.error);
  return fallback;
}
async function readInvite(token: string, options: RequestInit = {}) {
  const response = await fetch(`${apiOrigin()}/api/invite/${token}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      Origin: process.env.APP_ORIGIN || 'http://localhost:3000',
      ...options.headers,
    },
  });
  const data: unknown = await response.json().catch(() => null);
  return { response, data };
}
export async function loader({ params }: LoaderFunctionArgs) {
  const { response, data } = await readInvite(params.token || '');
  if (!response.ok) {
    throw json(
      { error: errorMessage(data, 'This invitation is no longer valid') },
      { status: response.status },
    );
  }
  return json(data as { email: string; role: string });
}
export async function action({ request, params }: ActionFunctionArgs) {
  requireOrigin(request);
  const form = await request.formData();
  const name = form.get('name');
  const password = form.get('password');
  const { response, data } = await readInvite(params.token || '', {
    method: 'POST',
    body: JSON.stringify({
      name: typeof name === 'string' ? name : '',
      password: typeof password === 'string' ? password : '',
    }),
  });
  if (!response.ok) {
    return json(
      { error: errorMessage(data, 'This invitation could not be accepted') },
      { status: response.status },
    );
  }
  const session = response.headers.get('Set-Cookie');
  if (!session) throw new Error('Invitation was accepted without a session');
  return redirect('/', { headers: { 'Set-Cookie': session } });
}
export default function AcceptInvite() {
  const invite = useLoaderData<typeof loader>();
  const data = useActionData<typeof action>();
  const navigation = useNavigation();
  return (
    <main className="login">
      <div className="login-brand">
        <span className="brand-mark">s</span>
        <b>sira</b>
        <p>A clearer view of every relationship.</p>
      </div>
      <Form method="post" className="login-form">
        <span className="eyebrow">YOUR INVITATION</span>
        <h1>Join the workspace.</h1>
        <p className="muted">
          {invite.email} will join as {roleLabel(invite.role).toLowerCase()}.
        </p>
        <label>
          Your name
          <input name="name" autoComplete="name" maxLength={80} required />
        </label>
        <label>
          Password
          <input name="password" type="password" autoComplete="new-password" required />
          <small>Use 14–72 characters.</small>
        </label>
        {data?.error && (
          <p role="alert" className="error">
            {data.error}
          </p>
        )}
        <button className="primary" disabled={navigation.state !== 'idle'}>
          {navigation.state !== 'idle' ? 'Creating account…' : 'Create account'}
        </button>
      </Form>
    </main>
  );
}
export function ErrorBoundary() {
  const error = useRouteError();
  const message = isRouteErrorResponse(error)
    ? errorMessage(error.data, 'This invitation is no longer valid')
    : 'This invitation is no longer valid';
  return (
    <main className="login">
      <div className="login-brand">
        <span className="brand-mark">s</span>
        <b>sira</b>
        <p>A clearer view of every relationship.</p>
      </div>
      <div className="login-form">
        <span className="eyebrow">YOUR INVITATION</span>
        <h1>This invitation is closed.</h1>
        <p role="alert" className="error">
          {message}
        </p>
        <a className="primary" href="/login">
          Sign in
        </a>
      </div>
    </main>
  );
}
