import { Form, useActionData, useNavigation } from '@remix-run/react';
import { json, redirect, type ActionFunctionArgs } from '@remix-run/node';
import { requireOrigin } from '../api.server';
export async function action({ request }: ActionFunctionArgs) {
  requireOrigin(request);
  const form = await request.formData();
  const r = await fetch(`${process.env.API_URL || 'http://localhost:8080'}/api/login`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Origin: process.env.APP_ORIGIN || 'http://localhost:3000',
    },
    body: JSON.stringify(Object.fromEntries(form)),
  });
  if (!r.ok)
    return json({ error: 'Sign-in failed. Check your email and password.' }, { status: r.status });
  return redirect('/', {
    headers: { 'Set-Cookie': r.headers.get('Set-Cookie')! },
  });
}
export default function Login() {
  const data = useActionData<typeof action>();
  const nav = useNavigation();
  return (
    <main className="login">
      <div className="login-brand">
        <span className="brand-mark">s</span>
        <b>sira</b>
        <p>A clearer view of every relationship.</p>
      </div>
      <Form method="post" className="login-form">
        <span className="eyebrow">YOUR WORKSPACE</span>
        <h1>Welcome back.</h1>
        <p className="muted">Sign in to your Sira workspace.</p>
        <label>
          Email
          <input name="email" type="email" autoComplete="username" required />
        </label>
        <label>
          Password
          <input name="password" type="password" autoComplete="current-password" required />
        </label>
        {data?.error && (
          <p role="alert" className="error">
            {data.error}
          </p>
        )}
        <button className="primary" disabled={nav.state !== 'idle'}>
          {nav.state !== 'idle' ? 'Signing in…' : 'Sign in'}
        </button>
        <p className="muted text-sm">Access is managed by your workspace administrator.</p>
      </Form>
    </main>
  );
}
