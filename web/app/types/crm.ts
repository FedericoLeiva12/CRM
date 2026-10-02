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
  manage_schema: boolean;
  permissions: Permission[];
}
export type Role = 'admin' | 'member';
export interface WorkspaceUser {
  id: string;
  email: string;
  name: string;
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
export type WorkspaceView = 'records' | 'fields' | 'agents' | 'team';
export type ModalKind = 'record' | 'field' | 'section' | 'agent' | 'delete' | 'revoke' | 'password';
export interface CreatedAgent {
  id: string;
  token: string;
}
