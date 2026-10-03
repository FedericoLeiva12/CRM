import type { WorkspaceView } from '../../types/crm';

export type SettingsView = Exclude<WorkspaceView, 'records'>;
export type SettingsGroupId = 'workspace' | 'connections' | 'account';

// Register settings here to keep navigation and server access checks in sync.
export interface SettingsGroup {
  id: SettingsGroupId;
  title: string;
  items: { view: SettingsView; label: string; adminOnly: boolean }[];
}

export const settingsGroups: SettingsGroup[] = [
  {
    id: 'workspace',
    title: 'Workspace',
    items: [
      { view: 'fields', label: 'Sections', adminOnly: true },
      { view: 'team', label: 'Team', adminOnly: true },
    ],
  },
  {
    id: 'connections',
    title: 'Connections',
    items: [
      { view: 'agents', label: 'Agent access', adminOnly: true },
      { view: 'webhooks', label: 'Webhooks', adminOnly: true },
    ],
  },
  {
    id: 'account',
    title: 'Account',
    items: [{ view: 'security', label: 'Security', adminOnly: false }],
  },
];

export function availableSettings(admin: boolean) {
  return settingsGroups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => admin || !item.adminOnly),
    }))
    .filter((group) => group.items.length > 0);
}
