import type { WorkspaceView } from '../../types/crm';

export type SettingsView = Exclude<WorkspaceView, 'records'>;

// Register settings here to keep navigation and server access checks in sync.
interface SettingsGroup {
  title: string;
  items: { view: SettingsView; label: string; adminOnly: boolean }[];
}

export const settingsGroups: SettingsGroup[] = [
  {
    title: 'Workspace',
    items: [
      { view: 'fields', label: 'Fields & sections', adminOnly: true },
      { view: 'team', label: 'Team', adminOnly: true },
    ],
  },
  {
    title: 'Connections',
    items: [
      { view: 'agents', label: 'Agent access', adminOnly: true },
      { view: 'webhooks', label: 'Webhooks', adminOnly: true },
    ],
  },
  { title: 'Account', items: [{ view: 'security', label: 'Security', adminOnly: false }] },
];

export function availableSettings(admin: boolean) {
  return settingsGroups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => admin || !item.adminOnly),
    }))
    .filter((group) => group.items.length > 0);
}
