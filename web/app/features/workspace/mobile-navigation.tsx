import { Form, Link } from '@remix-run/react';
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import { Contact, LogOut, Menu, Plus, Settings2, Users } from 'lucide-react';
import { useRef } from 'react';
import type { ModalKind, Section, WorkspaceView } from '../../types/crm';

interface Props {
  sections: Section[];
  section: Section;
  view: WorkspaceView;
  admin: boolean;
  onOpenModal: (modal: ModalKind) => void;
}

export function MobileNavigation({ sections, section, view, admin, onOpenModal }: Props) {
  const openingDialog = useRef(false);
  return (
    <div className="mobile-navigation">
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <button type="button" className="mobile-menu-trigger" aria-label="Open navigation menu">
            <Menu size={21} aria-hidden="true" />
          </button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            className="mobile-menu"
            align="start"
            sideOffset={8}
            collisionPadding={12}
            onCloseAutoFocus={(event) => {
              // The section dialog takes focus when opened from this menu.
              if (openingDialog.current) {
                event.preventDefault();
                openingDialog.current = false;
              }
            }}
          >
            <DropdownMenu.Label className="mobile-menu-brand">
              <span className="brand-mark" aria-hidden="true">
                S
              </span>
              <span>Sira CRM</span>
            </DropdownMenu.Label>
            <DropdownMenu.Group>
              {sections.map((option) => (
                <DropdownMenu.Item key={option.id} asChild>
                  <Link
                    to={`/?section=${option.id}`}
                    className="mobile-menu-item"
                    aria-current={
                      view === 'records' && section?.id === option.id ? 'page' : undefined
                    }
                  >
                    {option.id === 'prospects' ? (
                      <Contact size={18} aria-hidden="true" />
                    ) : (
                      <Users size={18} aria-hidden="true" />
                    )}
                    <span>{option.name}</span>
                  </Link>
                </DropdownMenu.Item>
              ))}
              {admin && (
                <DropdownMenu.Item
                  className="mobile-menu-item"
                  onSelect={() => {
                    openingDialog.current = true;
                    onOpenModal('section');
                  }}
                >
                  <Plus size={18} aria-hidden="true" />
                  Add section
                </DropdownMenu.Item>
              )}
            </DropdownMenu.Group>
            <DropdownMenu.Separator className="mobile-menu-separator" />
            <DropdownMenu.Item asChild>
              <Link
                className="mobile-menu-item"
                to={admin ? '/?view=fields' : '/?view=security'}
                aria-current={view !== 'records' ? 'page' : undefined}
              >
                <Settings2 size={18} aria-hidden="true" />
                Settings
              </Link>
            </DropdownMenu.Item>
            <Form method="post" action="/">
              <input type="hidden" name="intent" value="logout" />
              <DropdownMenu.Item asChild>
                <button type="submit" className="mobile-menu-item">
                  <LogOut size={18} aria-hidden="true" />
                  Sign out
                </button>
              </DropdownMenu.Item>
            </Form>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div>
  );
}
