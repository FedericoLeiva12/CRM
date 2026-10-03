import { Form, Link } from '@remix-run/react';
import * as Tooltip from '@radix-ui/react-tooltip';
import { useState, type ReactElement } from 'react';
import {
  Contact,
  LogOut,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Settings2,
  Users,
} from 'lucide-react';
import type { ModalKind, Section, WorkspaceView } from '../../types/crm';

interface Props {
  sections: Section[];
  section: Section;
  view: WorkspaceView;
  admin: boolean;
  initiallyCollapsed: boolean;
  onOpenModal: (modal: ModalKind) => void;
}

function SidebarTooltip({
  label,
  enabled,
  children,
}: {
  label: string;
  enabled: boolean;
  children: ReactElement;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Tooltip.Root open={enabled && open} onOpenChange={setOpen}>
      <Tooltip.Trigger asChild>{children}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content
          className="sidebar-tooltip"
          side="right"
          sideOffset={10}
          collisionPadding={12}
        >
          {label}
          <Tooltip.Arrow className="sidebar-tooltip-arrow" />
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}

export function Sidebar({
  sections,
  section,
  view,
  admin,
  initiallyCollapsed,
  onOpenModal,
}: Props) {
  const [collapsed, setCollapsed] = useState(initiallyCollapsed);
  const toggleLabel = collapsed ? 'Expand sidebar' : 'Collapse sidebar';
  function toggleSidebar() {
    const next = !collapsed;
    setCollapsed(next);
    // Read by the loader on the next visit, avoiding a flash of the expanded sidebar.
    document.cookie = `sira_sidebar=${next ? 'collapsed' : 'expanded'}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === 'https:' ? '; Secure' : ''}`;
  }
  return (
    <Tooltip.Provider delayDuration={200}>
      <aside className={`sidebar ${collapsed ? 'sidebar-collapsed' : ''}`}>
        <div className="sidebar-heading">
          <SidebarTooltip label="Sira CRM" enabled={collapsed}>
            <Link to="/" className="brand" aria-label="Sira CRM">
              <span className="brand-mark" aria-hidden="true">
                S
              </span>
              <span className="brand-name">
                sira<span className="brand-crm">CRM</span>
              </span>
            </Link>
          </SidebarTooltip>
          <SidebarTooltip label={toggleLabel} enabled>
            <button
              type="button"
              className="sidebar-toggle"
              aria-label={toggleLabel}
              aria-expanded={!collapsed}
              aria-controls="workspace-navigation"
              onClick={toggleSidebar}
            >
              {collapsed ? (
                <PanelLeftOpen size={18} aria-hidden="true" />
              ) : (
                <PanelLeftClose size={18} aria-hidden="true" />
              )}
            </button>
          </SidebarTooltip>
        </div>
        <div className="nav-label">WORKSPACE</div>
        <nav id="workspace-navigation" aria-label="Workspace sections">
          {sections.map((sectionOption) => (
            <SidebarTooltip key={sectionOption.id} label={sectionOption.name} enabled={collapsed}>
              <Link
                to={`/?section=${sectionOption.id}`}
                aria-label={sectionOption.name}
                aria-current={
                  view === 'records' && section?.id === sectionOption.id ? 'page' : undefined
                }
                className={`nav-item ${view === 'records' && section?.id === sectionOption.id ? 'active' : ''}`}
              >
                {sectionOption.id === 'prospects' ? (
                  <Contact size={18} aria-hidden="true" />
                ) : (
                  <Users size={18} aria-hidden="true" />
                )}
                <span className="sidebar-item-label">{sectionOption.name}</span>
              </Link>
            </SidebarTooltip>
          ))}
          {admin && (
            <SidebarTooltip label="Add section" enabled={collapsed}>
              <button
                className="nav-item add-section"
                aria-label="Add section"
                onClick={() => onOpenModal('section')}
              >
                <Plus size={17} aria-hidden="true" />
                <span className="sidebar-item-label">Add section</span>
              </button>
            </SidebarTooltip>
          )}
        </nav>
        <SidebarTooltip label="Settings" enabled={collapsed}>
          <Link
            to={admin ? '/?view=fields' : '/?view=security'}
            aria-label="Settings"
            className={`nav-item settings-entry ${view !== 'records' ? 'active' : ''}`}
            aria-current={view !== 'records' ? 'page' : undefined}
          >
            <Settings2 size={18} aria-hidden="true" />
            <span className="sidebar-item-label">Settings</span>
          </Link>
        </SidebarTooltip>
        <Form method="post">
          <input type="hidden" name="intent" value="logout" />
          <SidebarTooltip label="Sign out" enabled={collapsed}>
            <button className="nav-item logout" aria-label="Sign out">
              <LogOut size={17} aria-hidden="true" />
              <span className="sidebar-item-label">Sign out</span>
            </button>
          </SidebarTooltip>
        </Form>
      </aside>
    </Tooltip.Provider>
  );
}
