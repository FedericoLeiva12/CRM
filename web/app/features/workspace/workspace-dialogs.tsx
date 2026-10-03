import { Form } from '@remix-run/react';
import { Modal } from '../../components/modal';
import type { Agent, CRMRecord, ModalKind, Section } from '../../types/crm';
import { dialogPresentation } from './presentation';
import {
  AgentInputs,
  CustomFieldInputs,
  PasswordInputs,
  RecordFields,
  SectionInputs,
} from './editor-fields';
interface Props {
  modal: ModalKind | null;
  editing: CRMRecord | null;
  revoke: Agent | null;
  section: Section;
  busy: boolean;
  error?: string;
  onClose: () => void;
}
export function WorkspaceDialogs({ modal, editing, revoke, section, busy, error, onClose }: Props) {
  if (!modal) return null;
  const presentation = dialogPresentation[modal];
  // Existing items open the detail screen. This dialog creates records and handles small workspace actions.
  const actions = (
    <div className="modal-actions">
      <button type="button" className="secondary" onClick={onClose}>
        Cancel
      </button>
      <button className={presentation.destructive ? 'danger' : 'primary'} disabled={busy}>
        {busy ? 'Saving…' : presentation.submitLabel}
      </button>
    </div>
  );
  return (
    <Modal
      title={presentation.title}
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      <Form method="post" className="modal-form">
        <input type="hidden" name="intent" value={presentation.intent} />
        <input type="hidden" name="section" value={section.id} />
        {modal === 'record' && <RecordFields fields={section.fields} record={editing} />}
        {modal === 'field' && <CustomFieldInputs />}
        {modal === 'section' && <SectionInputs />}
        {modal === 'agent' && <AgentInputs />}
        {modal === 'password' && <PasswordInputs />}
        {modal === 'delete' && (
          <>
            <input type="hidden" name="id" value={editing?.id} />
            <p>
              <b>{String(editing?.data.name)}</b> will be permanently removed from{' '}
              {section.name.toLowerCase()}.
            </p>
          </>
        )}
        {modal === 'revoke' && (
          <>
            <input type="hidden" name="id" value={revoke?.id} />
            <p>
              <b>{revoke?.name}</b> will immediately lose access. This token cannot be restored.
            </p>
          </>
        )}
        {actions}
      </Form>
    </Modal>
  );
}
