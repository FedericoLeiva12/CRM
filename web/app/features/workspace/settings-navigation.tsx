import { Link, useNavigate } from '@remix-run/react';
import { Bot, Settings2, ShieldCheck, UserPlus, Webhook } from 'lucide-react';
import { availableSettings, type SettingsView } from './settings';

const icons = {
  fields: Settings2,
  team: UserPlus,
  agents: Bot,
  webhooks: Webhook,
  security: ShieldCheck,
};

export function SettingsNavigation({ view, admin }: { view: SettingsView; admin: boolean }) {
  const groups = availableSettings(admin);
  const navigate = useNavigate();
  return (
    <aside className="settings-navigation">
      <h2>Settings</h2>
      <nav aria-label="Settings" className="settings-desktop-nav">
        {groups.map((group) => (
          <div className="settings-group" key={group.title}>
            <h3>{group.title}</h3>
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
          </div>
        ))}
      </nav>
      <div className="settings-mobile-nav">
        <label htmlFor="settings-section">Settings section</label>
        <select
          id="settings-section"
          value={view}
          onChange={(event) => navigate(`/?view=${event.target.value}`)}
        >
          {groups.map((group) => (
            <optgroup key={group.title} label={group.title}>
              {group.items.map((item) => (
                <option key={item.view} value={item.view}>
                  {item.label}
                </option>
              ))}
            </optgroup>
          ))}
        </select>
      </div>
    </aside>
  );
}
