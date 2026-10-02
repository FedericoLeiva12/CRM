import { realpathSync } from 'node:fs';
import { vitePlugin as remix } from '@remix-run/dev';
import { defineConfig, searchForWorkspaceRoot } from 'vite';

export default defineConfig({
  plugins: [remix()],
  server: {
    port: 3000,
    fs: {
      // Support dependencies stored outside cloud-synced workspaces through a symlink.
      allow: [searchForWorkspaceRoot(process.cwd()), realpathSync('node_modules')],
    },
  },
});
