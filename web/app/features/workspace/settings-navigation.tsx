import * as Tabs from '@radix-ui/react-tabs';
import { Link, useNavigate } from '@remix-run/react';
import { Bot, Settings2, ShieldCheck, UserPlus, Webhook } from 'lucide-react';
import { useEffect, useRef, type ReactNode } from 'react';
import type { WorkspaceView } from '../../types/crm';
import {
  availableSettings,
  type SettingsGroup,
  type SettingsGroupId,
  type SettingsView,
} from './settings';

const icons = {
  fields: Settings2,
  team: UserPlus,
  agents: Bot,
  webhooks: Webhook,
  security: ShieldCheck,
};

// The route is the source of truth for both navigation levels, including deep links and Back.
export function WorkspacePage({
  view,
  admin,
  className,
  children,
}: {
  view: WorkspaceView;
  admin: boolean;
  className: string;
  children: ReactNode;
}) {
  const groups = availableSettings(admin);
  const activeGroup = groups.find((group) => group.items.some((item) => item.view === view));
  const groupId = activeGroup?.id;
  const lastVisited = useRef<Partial<Record<SettingsGroupId, SettingsView>>>({});
  const navigate = useNavigate();

  useEffect(() => {
    if (groupId && view !== 'records') lastVisited.current[groupId] = view;
  }, [groupId, view]);

  if (!activeGroup) return <div className={`page ${className}`}>{children}</div>;

  function selectGroup(id: string) {
    const group = groups.find((candidate) => candidate.id === id);
    if (!group) return;
    const remembered = lastVisited.current[group.id];
    // Recheck availability so remembered destinations cannot bypass role filtering.
    const destination = group.items.find((item) => item.view === remembered) ?? group.items[0];
    navigate(`/?view=${destination.view}`);
  }

  return (
    <Tabs.Root
      className="page settings-layout"
      value={activeGroup.id}
      onValueChange={selectGroup}
      activationMode="manual"
    >
      <h1 className="settings-heading">Settings</h1>
      <Tabs.List aria-label="Settings categories" className="settings-category-tabs">
        {groups.map((group) => (
          <Tabs.Trigger key={group.id} value={group.id}>
            {group.title}
          </Tabs.Trigger>
        ))}
      </Tabs.List>
      {groups.map((group) => (
        <Tabs.Content key={group.id} value={group.id} className="settings-category-panel">
          {group.id === activeGroup.id && (
            <div className="settings-body">
              <SettingsNavigation group={group} view={view as SettingsView} />
              {children}
            </div>
          )}
        </Tabs.Content>
      ))}
    </Tabs.Root>
  );
}

function SettingsNavigation({ group, view }: { group: SettingsGroup; view: SettingsView }) {
  return (
    <aside className="settings-navigation">
      <nav aria-label={`${group.title} settings`} className="settings-section-nav">
        {group.items.map((item) => {
          const Icon = icons[item.view];
          return (
            <Link
              key={item.view}
              to={`/?view=${item.view}`}
              aria-current={view === item.view ? 'page' : undefined}
              className="settings-link"
            >
              <Icon size={17} aria-hidden="true" />
              {item.label}
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
