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
export interface Permission {
  section_id: string;
  read: boolean;
  write: boolean;
}
export interface Agent {
  id: string;
  name: string;
  permissions: Permission[];
}
export type WorkspaceView = 'records' | 'fields' | 'agents';
export type ModalKind = 'record' | 'field' | 'section' | 'agent' | 'delete' | 'revoke' | 'password';
export interface CreatedAgent {
  id: string;
  token: string;
}
