import { Form } from '@remix-run/react';
import { Bot, Plus, ShieldCheck } from 'lucide-react';
import { SectionPermissionRow } from './section-permission-row';
import { CheckControl } from '../../components/permission-checkbox';
import type { Agent, Section } from '../../types/crm';
interface Props {
  agents: Agent[];
  sections: Section[];
  token: string;
  busy: boolean;
  onDismissToken: () => void;
  onCreateAgent: () => void;
  onRevokeAgent: (agent: Agent) => void;
}
export function AgentsView({
  agents,
  sections,
  token,
  busy,
  onDismissToken,
  onCreateAgent,
  onRevokeAgent,
}: Props) {
  return (
    <>
      <div className="agent-intro">
        <ShieldCheck size={24} />
        <div>
          <h2>You set the boundaries.</h2>
          <p>
            Read access lets agents retrieve records. Write access lets them create and edit
            records, and always includes Read. Delete access separately permits permanent record
            deletion. Manage schema lets them manage section definitions and item views. A new
            section stays closed for every agent, including the one that created it, until you grant
            read or write.
          </p>
        </div>
        <code>/mcp</code>
      </div>
      {token && (
        <div className="token-panel">
          <h2>Save your agent token</h2>
          <p>
            This token is shown once. Store it securely and use it as a Bearer token at your
            workspace’s <code>/mcp</code> endpoint.
          </p>
          <code className="token-value">{token}</code>
          <button className="secondary" onClick={() => onDismissToken()}>
            I have saved it
          </button>
        </div>
      )}
      {agents.length === 0 ? (
        <div className="empty agents-empty">
          <Bot size={34} />
          <h2>Bring your agents into the loop.</h2>
          <p>Create an agent token, then choose which sections it can access.</p>
          <button className="primary" onClick={() => onCreateAgent()}>
            <Plus size={17} />
            Connect your first agent
          </button>
        </div>
      ) : (
        agents.map((agent) => (
          <Form method="post" className="agent-panel" key={agent.id}>
            <input type="hidden" name="intent" value="permissions" />
            <input type="hidden" name="id" value={agent.id} />
            <div className="panel-heading">
              <div className="agent-heading">
                <Bot size={21} />
                <h2>{agent.name}</h2>
              </div>
              <button
                type="button"
                className="danger-text"
                onClick={() => {
                  onRevokeAgent(agent);
                }}
              >
                Revoke token
              </button>
            </div>
            <div className="schema-permission">
              <div>
                <b>Manage schema</b>
                <p>
                  Discover section definitions, create sections, add fields and configure item
                  views. Does not include record access.
                </p>
              </div>
              <CheckControl
                name="manage_schema"
                checked={agent.manage_schema}
                disabled={busy}
                label={`Manage schema for ${agent.name}`}
              />
            </div>
            <div className="permissions-header">
              <span>Section</span>
              <span>Read</span>
              <span>Write</span>
              <span>Delete</span>
            </div>
            {agent.permissions.map((permission) => (
              <SectionPermissionRow
                key={`${permission.section_id}:${permission.read}:${permission.write}:${permission.delete}`}
                permission={permission}
                name={
                  sections.find((candidate) => candidate.id === permission.section_id)?.name ||
                  permission.section_id
                }
                disabled={busy}
              />
            ))}
            <div className="agent-save">
              <span className="muted">Changes apply to subsequent agent requests.</span>
              <button className="primary" disabled={busy}>
                Save permissions
              </button>
            </div>
          </Form>
        ))
      )}
    </>
  );
}
