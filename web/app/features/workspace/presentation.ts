import type { ModalKind, Section, WorkspaceView } from '../../types/crm';

const recordLabels: Record<string, string> = { clients: 'client', prospects: 'prospect' };
export function viewPresentation(view: WorkspaceView, section: Section) {
  if (view === 'agents')
    return {
      title: 'Agent access',
      description: 'Give each agent exactly the access it needs.',
      actionLabel: 'Connect agent',
      actionModal: 'agent' as ModalKind,
    };
  if (view === 'team')
    return {
      title: 'Team',
      description: 'Invite people and choose what they can manage.',
      actionLabel: '',
      actionModal: 'record' as ModalKind,
    };
  if (view === 'webhooks')
    return {
      title: 'Webhooks',
      description: 'Send workspace events to the systems you choose.',
      actionLabel: 'Add webhook',
      actionModal: 'record' as ModalKind,
    };
  if (view === 'fields')
    return {
      title: 'Fields & sections',
      description: 'Shape your workspace around the way you work.',
      actionLabel: 'Add section',
      actionModal: 'section' as ModalKind,
    };
  return {
    title: section.name,
    description:
      section.id === 'prospects'
        ? 'Turn your next conversation into a lasting relationship.'
        : 'Your relationships, with the details that matter.',
    actionLabel: `Add ${recordLabels[section.id] || 'record'}`,
    actionModal: 'record' as ModalKind,
  };
}
export const actionMessages: Record<string, string> = {
  permissions: 'Permissions saved.',
  save: 'Relationship saved.',
  field: 'Field added.',
  section: 'Section created.',
  delete: 'Record deleted.',
  revoke: 'Agent revoked.',
  invite: 'Invitation link created.',
  'revoke-invite': 'Invitation revoked.',
  role: 'Role updated.',
  'remove-user': 'Person removed.',
  'webhook-create': 'Webhook saved.',
  'webhook-update': 'Webhook saved.',
  'webhook-delete': 'Webhook deleted.',
  'webhook-enable': 'Webhook enabled.',
  'webhook-disable': 'Webhook disabled.',
  'webhook-test': 'Test event queued.',
};
export const webhookEvents = [
  { id: 'record.created', label: 'Record created' },
  { id: 'record.updated', label: 'Record updated' },
  { id: 'record.deleted', label: 'Record deleted' },
  { id: 'section.created', label: 'Section created' },
  { id: 'field.created', label: 'Field created' },
  { id: 'timeline.entry_created', label: 'Timeline entry' },
] as const;
const extraEventLabels: Record<string, string> = { 'webhook.test': 'Test' };
export function webhookEventLabel(eventType: string) {
  return (
    webhookEvents.find((event) => event.id === eventType)?.label ||
    extraEventLabels[eventType] ||
    eventType
  );
}
export function roleLabel(role: string) {
  return role === 'admin' ? 'Administrator' : 'Member';
}
export const dialogPresentation: Record<
  ModalKind,
  { title: string; submitLabel: string; intent: string; destructive: boolean }
> = {
  record: {
    title: 'Add relationship',
    submitLabel: 'Save relationship',
    intent: 'save',
    destructive: false,
  },
  field: {
    title: 'Add a custom field',
    submitLabel: 'Add field',
    intent: 'field',
    destructive: false,
  },
  section: {
    title: 'Add a section',
    submitLabel: 'Create section',
    intent: 'section',
    destructive: false,
  },
  agent: {
    title: 'Connect an agent',
    submitLabel: 'Create token',
    intent: 'agent',
    destructive: false,
  },
  delete: {
    title: 'Delete this record?',
    submitLabel: 'Delete record',
    intent: 'delete',
    destructive: true,
  },
  revoke: {
    title: 'Revoke this agent?',
    submitLabel: 'Revoke token',
    intent: 'revoke',
    destructive: true,
  },
  password: {
    title: 'Change password',
    submitLabel: 'Change password',
    intent: 'password',
    destructive: false,
  },
};
