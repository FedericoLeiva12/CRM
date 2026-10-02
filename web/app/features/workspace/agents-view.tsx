import { Form } from '@remix-run/react';
import { Bot, Plus, ShieldCheck } from 'lucide-react';
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
            Read access lets agents retrieve records. Write access lets them create, replace, and
            delete records. New sections are always disabled until granted.
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
            <div className="permissions-header">
              <span>Section</span>
              <span>Read</span>
              <span>Write</span>
            </div>
            {agent.permissions.map((permission) => (
              <div className="permission-row" key={permission.section_id}>
                <span>
                  {
                    sections.find((sectionOption) => sectionOption.id === permission.section_id)
                      ?.name
                  }
                </span>
                <CheckControl
                  name={`${permission.section_id}:read`}
                  checked={permission.read}
                  label={`Read ${permission.section_id}`}
                />
                <CheckControl
                  name={`${permission.section_id}:write`}
                  checked={permission.write}
                  label={`Write ${permission.section_id}`}
                />
              </div>
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
