import { Links, Meta, Outlet, Scripts, ScrollRestoration } from '@remix-run/react';
import './styles.css';
export const meta = () => [
  { title: 'Sira CRM' },
  { name: 'description', content: 'Your private relationship workspace' },
];
export default function App() {
  return (
    <html lang="en">
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" href="/favicon.svg" type="image/svg+xml" />
        <Meta />
        <Links />
      </head>
      <body>
        <Outlet />
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}
