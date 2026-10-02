import { Form } from '@remix-run/react';
import { Modal } from '../../components/modal';
import type { Agent, CRMRecord, ModalKind, Section, Viewer } from '../../types/crm';
import { dialogPresentation } from './presentation';
import {
  AgentInputs,
  CustomFieldInputs,
  PasswordInputs,
  RecordFields,
  SectionInputs,
} from './editor-fields';
import { RecordHistory } from './record-history';
interface Props {
  modal: ModalKind | null;
  editing: CRMRecord | null;
  revoke: Agent | null;
  section: Section;
  viewer: Viewer;
  busy: boolean;
  error?: string;
  onClose: () => void;
}
export function WorkspaceDialogs({
  modal,
  editing,
  revoke,
  section,
  viewer,
  busy,
  error,
  onClose,
}: Props) {
  if (!modal) return null;
  const presentation = dialogPresentation[modal];
  const title = modal === 'record' && editing ? 'Edit relationship' : presentation.title;
  // The history has its own forms, and forms cannot nest. For an existing record it sits
  // between the record form and the action buttons, which then submit the form by id.
  const withHistory = modal === 'record' && editing !== null;
  const formId = 'dialog-form';
  const actions = (
    <div className="modal-actions">
      <button type="button" className="secondary" onClick={onClose}>
        Cancel
      </button>
      <button
        className={presentation.destructive ? 'danger' : 'primary'}
        disabled={busy}
        form={withHistory ? formId : undefined}
      >
        {busy ? 'Saving…' : presentation.submitLabel}
      </button>
    </div>
  );
  return (
    <Modal
      title={title}
      open
      wide={withHistory}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      <Form method="post" className="modal-form" id={formId}>
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
        {!withHistory && actions}
      </Form>
      {withHistory && editing && (
        <RecordHistory
          key={editing.id}
          sectionID={section.id}
          recordID={editing.id}
          viewer={viewer}
        />
      )}
      {withHistory && actions}
    </Modal>
  );
}
