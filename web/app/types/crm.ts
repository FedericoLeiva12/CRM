export type FieldType = 'text' | 'email' | 'number' | 'date' | 'boolean';
export interface Field {
  id: string;
  label: string;
  type: FieldType;
  required: boolean;
}
export interface Section {
  id: string;
  name: string;
  fields: Field[];
}
export interface CRMRecord {
  id: string;
  data: Record<string, string | number | boolean | null>;
  updated_at: string;
}
export interface RecordPage {
  records: CRMRecord[];
  total: number;
  nextCursor: string | null;
  error: string | null;
}
export interface RecordLink {
  section_id: string;
  section_name: string;
  record_id: string;
  direction: 'outgoing' | 'incoming';
  name?: string;
}
export interface ActivityAuthor {
  kind: 'user' | 'agent';
  id: string;
  name?: string;
  handle?: string;
}
export interface Principal {
  kind: 'user' | 'agent';
  id: string;
  handle: string;
  name?: string;
}
export interface Activity {
  id: string;
  type: string;
  date: string;
  summary: string;
  channel?: string;
  ref?: string;
  author: ActivityAuthor;
  created_at: string;
  parent_id?: string;
  edited_at?: string;
  deleted?: boolean;
  mentions?: Principal[];
}
export interface MentionNotification {
  id: string;
  entry_id: string;
  section_id: string;
  section_name: string;
  record_id: string;
  record_name?: string;
  parent_id?: string;
  author: ActivityAuthor;
  body: string;
  created_at: string;
  read_at: string | null;
}
export interface MentionCandidates {
  users: Principal[];
  agents: Principal[];
}
export interface Viewer {
  id: string;
  role: Role;
}
export interface RecordDetail extends CRMRecord {
  links: RecordLink[];
  activities: Activity[];
}
export interface Permission {
  section_id: string;
  read: boolean;
  write: boolean;
}
export interface Agent {
  id: string;
  name: string;
  handle: string;
  manage_schema: boolean;
  permissions: Permission[];
}
export type Role = 'admin' | 'member';
export interface WorkspaceUser {
  id: string;
  email: string;
  name: string;
  handle: string;
  role: Role;
  created_at: string;
}
export interface Invite {
  id: string;
  email: string;
  role: Role;
  created_at: string;
  expires_at: string;
}
export interface CreatedInvite extends Invite {
  token: string;
  link: string;
}
export type WorkspaceView = 'records' | 'fields' | 'agents' | 'team' | 'webhooks';
export type ModalKind = 'record' | 'field' | 'section' | 'agent' | 'delete' | 'revoke' | 'password';
export interface WebhookActorRef {
  kind: 'user' | 'agent';
  id: string;
  name?: string;
}
export interface WebhookEndpoint {
  id: string;
  url: string;
  description: string;
  event_types: string[];
  section_id: string | null;
  enabled: boolean;
  auto_disabled: boolean;
  consecutive_failures: number;
  skipped_events: number;
  excluded_actors: WebhookActorRef[];
  signing_secret_set: boolean;
  custom_header_name: string;
  custom_header_set: boolean;
  created_at: string;
  updated_at: string;
  failure_limit: number;
  max_attempts: number;
}
export interface WebhookDelivery {
  id: number;
  event_id: string;
  event_type: string;
  status: string;
  attempt_count: number;
  status_code: number | null;
  latency_ms: number | null;
  response: string;
  next_attempt_at: string;
  created_at: string;
  updated_at: string;
  max_attempts: number;
}
export interface CreatedAgent {
  id: string;
  token: string;
}
