import { Form, useActionData } from '@remix-run/react';
import { Copy, Link2, UserPlus } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Modal } from '../../components/modal';
import type { Invite, WorkspaceUser } from '../../types/crm';
import type { workspaceAction } from './workspace.server';
import { roleLabel } from './presentation';

interface Props {
  users: WorkspaceUser[];
  invites: Invite[];
  currentUserId: string;
  inviteLink: string;
  busy: boolean;
  onDismissLink: () => void;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('en', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }).format(date);
}

export function TeamView({
  users,
  invites,
  currentUserId,
  inviteLink,
  busy,
  onDismissLink,
}: Props) {
  const actionResult = useActionData<typeof workspaceAction>();
  const [copied, setCopied] = useState(false);
  const [removing, setRemoving] = useState<WorkspaceUser | null>(null);
  useEffect(() => {
    setCopied(false);
  }, [inviteLink]);
  useEffect(() => {
    if (actionResult?.ok && actionResult.intent === 'remove-user') setRemoving(null);
  }, [actionResult]);
  async function copyLink() {
    try {
      await navigator.clipboard.writeText(inviteLink);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }
  const removeError =
    actionResult && !actionResult.ok && actionResult.intent === 'remove-user'
      ? actionResult.error
      : undefined;
  const formKey = actionResult
    ? `${actionResult.intent}:${String(actionResult.ok)}:${actionResult.error || ''}`
    : 'ready';
  return (
    <>
      <div className="agent-intro">
        <UserPlus size={24} />
        <div>
          <h2>Share a link, not an inbox.</h2>
          <p>
            Create an invitation, copy the link, and send it yourself. Sira does not email anyone.
            The link is shown once, works one time, and expires after 7 days. Members can view and
            edit records. Administrators also manage the team, agents, and the shape of the
            workspace.
          </p>
        </div>
      </div>
      <Form method="post" className="invite-form">
        <input type="hidden" name="intent" value="invite" />
        <label>
          Email
          <input name="email" type="email" autoComplete="off" required />
        </label>
        <label>
          Role
          <select name="role" defaultValue="member">
            <option value="member">Member</option>
            <option value="admin">Administrator</option>
          </select>
        </label>
        <button className="primary" disabled={busy}>
          <Link2 size={16} />
          Create invite link
        </button>
      </Form>
      {inviteLink && (
        <div className="token-panel">
          <h2>Copy this invitation link</h2>
          <p>
            This link is shown once. Share it with the person you invited. It cannot be retrieved
            later.
          </p>
          <code className="token-value">{inviteLink}</code>
          <div className="token-actions">
            <button type="button" className="secondary" onClick={copyLink}>
              <Copy size={16} />
              {copied ? 'Copied' : 'Copy link'}
            </button>
            <button type="button" className="secondary" onClick={onDismissLink}>
              I have saved it
            </button>
          </div>
          {copied && (
            <p role="status" className="success-message">
              Link copied.
            </p>
          )}
        </div>
      )}
      <section className="team-panel">
        <div className="panel-heading">
          <h2>Pending invitations</h2>
        </div>
        {invites.length === 0 ? (
          <p className="team-empty">No invitations waiting. Create a link and share it directly.</p>
        ) : (
          invites.map((invite) => {
            const expired = new Date(invite.expires_at).getTime() <= Date.now();
            return (
              <div className="person-row" key={invite.id}>
                <div className="person-meta">
                  <b>{invite.email}</b>
                  <span>
                    {roleLabel(invite.role)}
                    {expired ? ' · Expired' : ` · Expires ${formatDate(invite.expires_at)}`}
                  </span>
                </div>
                <Form method="post">
                  <input type="hidden" name="intent" value="revoke-invite" />
                  <input type="hidden" name="id" value={invite.id} />
                  <button className="danger-text" disabled={busy}>
                    Revoke
                  </button>
                </Form>
              </div>
            );
          })
        )}
      </section>
      <section className="team-panel">
        <div className="panel-heading">
          <h2>People</h2>
        </div>
        {users.map((user) => (
          <div className="person-row" key={user.id}>
            <div className="person-meta">
              <b>{user.name || user.email}</b>
              {user.name ? <span>{user.email}</span> : null}
              <small>Joined {formatDate(user.created_at)}</small>
            </div>
            <Form method="post" className="role-form" key={`${user.id}-${user.role}-${formKey}`}>
              <input type="hidden" name="intent" value="role" />
              <input type="hidden" name="id" value={user.id} />
              <label>
                <span className="sr-only">Role for {user.email}</span>
                <select name="role" defaultValue={user.role} aria-label={`Role for ${user.email}`}>
                  <option value="member">Member</option>
                  <option value="admin">Administrator</option>
                </select>
              </label>
              <button className="secondary" disabled={busy}>
                Save role
              </button>
            </Form>
            {user.id !== currentUserId && (
              <button
                type="button"
                className="danger-text"
                onClick={() => setRemoving(user)}
                disabled={busy}
              >
                Remove
              </button>
            )}
          </div>
        ))}
      </section>
      {removing && (
        <Modal
          title="Remove this person?"
          open
          onOpenChange={(open) => {
            if (!open) setRemoving(null);
          }}
        >
          {removeError && (
            <p role="alert" className="error">
              {removeError}
            </p>
          )}
          <Form method="post" className="modal-form">
            <input type="hidden" name="intent" value="remove-user" />
            <input type="hidden" name="id" value={removing.id} />
            <p>
              <b>{removing.name || removing.email}</b> will lose access immediately. Their sessions
              end now.
            </p>
            <div className="modal-actions">
              <button type="button" className="secondary" onClick={() => setRemoving(null)}>
                Cancel
              </button>
              <button className="danger" disabled={busy}>
                {busy ? 'Removing…' : 'Remove person'}
              </button>
            </div>
          </Form>
        </Modal>
      )}
    </>
  );
}
