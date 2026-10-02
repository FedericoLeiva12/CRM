import { Form, Link } from '@remix-run/react';
import {
  Bot,
  ChevronRight,
  Contact,
  LayoutGrid,
  LogOut,
  Plus,
  Settings2,
  ShieldCheck,
  Users,
} from 'lucide-react';
import type { ModalKind, Section, WorkspaceView } from '../../types/crm';
interface Props {
  sections: Section[];
  section: Section;
  view: WorkspaceView;
  onOpenModal: (modal: ModalKind) => void;
}
export function Sidebar({ sections, section, view, onOpenModal }: Props) {
  return (
    <aside className="sidebar">
      <Link to="/" className="brand">
        <span className="brand-mark">s</span>sira
        <span className="brand-crm">CRM</span>
      </Link>
      <div className="workspace-label">
        <span className="workspace-avatar">S</span>
        <div>
          <b>Sira workspace</b>
          <small>Relationship management</small>
        </div>
      </div>
      <div className="nav-label">WORKSPACE</div>
      <nav>
        <Link to="/" className={view === 'records' ? 'nav-item home-link' : 'nav-item'}>
          <LayoutGrid size={18} />
          Overview
          <ChevronRight size={14} />
        </Link>
        {sections.map((s) => (
          <Link
            key={s.id}
            to={`/?section=${s.id}`}
            className={`nav-item ${view === 'records' && section?.id === s.id ? 'active' : ''}`}
          >
            {s.id === 'prospects' ? <Contact size={18} /> : <Users size={18} />}
            <span>{s.name}</span>
          </Link>
        ))}
        <button className="nav-item add-section" onClick={() => onOpenModal('section')}>
          <Plus size={17} />
          Add section
        </button>
      </nav>
      <div className="nav-label settings-label">CONTROL CENTER</div>
      <Link to="/?view=fields" className={`nav-item ${view === 'fields' ? 'active' : ''}`}>
        <Settings2 size={18} />
        Fields & sections
      </Link>
      <Link to="/?view=agents" className={`nav-item ${view === 'agents' ? 'active' : ''}`}>
        <Bot size={18} />
        Agent access
      </Link>
      <button className="nav-item" onClick={() => onOpenModal('password')}>
        <ShieldCheck size={18} />
        Account security
      </button>
      <div className="sidebar-footer">
        <ShieldCheck size={18} />
        <span>
          Private workspace<small>Controlled agent access</small>
        </span>
      </div>
      <Form method="post">
        <input type="hidden" name="intent" value="logout" />
        <button className="nav-item logout">
          <LogOut size={17} />
          Sign out
        </button>
      </Form>
    </aside>
  );
}
