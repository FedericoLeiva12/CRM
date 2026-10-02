import {
  useLoaderData,
  useActionData,
  useNavigation,
  useSearchParams,
  useRouteError,
  isRouteErrorResponse,
} from '@remix-run/react';
import { viewPresentation, actionMessages } from '../features/workspace/presentation';
import { ChevronRight, Plus } from 'lucide-react';
import { useEffect, useState } from 'react';
import { workspaceLoader, workspaceAction } from '../features/workspace/workspace.server';
import { Sidebar } from '../features/workspace/sidebar';
import { RecordsView } from '../features/workspace/records-view';
import { FieldsView } from '../features/workspace/fields-view';
import { AgentsView } from '../features/workspace/agents-view';
import { TeamView } from '../features/workspace/team-view';
import { WorkspaceDialogs } from '../features/workspace/workspace-dialogs';
import type { Agent, CRMRecord, ModalKind } from '../types/crm';
export const loader = workspaceLoader;
export const action = workspaceAction;
export default function Workspace() {
  const { sections, section, view, records, agents, users, invites, currentUser } =
    useLoaderData<typeof loader>();
  const actionResult = useActionData<typeof action>();
  const navigation = useNavigation();
  const [, setParams] = useSearchParams();
  const [modal, setModal] = useState<ModalKind | null>(null);
  const [editing, setEditing] = useState<CRMRecord | null>(null);
  const [revoke, setRevoke] = useState<Agent | null>(null);
  const [token, setToken] = useState('');
  const [inviteLink, setInviteLink] = useState('');
  const busy = navigation.state !== 'idle';
  useEffect(() => {
    if (actionResult?.ok) {
      setModal(null);
      if (actionResult.token) setToken(actionResult.token);
      if (actionResult.inviteLink) setInviteLink(actionResult.inviteLink);
    }
  }, [actionResult]);
  const error = actionResult?.error;
  const presentation = viewPresentation(view, section);
  function openPrimaryAction() {
    if (presentation.actionModal === 'record') openRecord(null);
    else setModal(presentation.actionModal);
  }
  function openRecord(record: CRMRecord | null) {
    setEditing(record);
    setModal('record');
  }
  return (
    <div className="workspace">
      <Sidebar
        sections={sections}
        section={section}
        view={view}
        admin={currentUser.role === 'admin'}
        onOpenModal={setModal}
      />

      <main className="main" aria-busy={busy}>
        <header className="topbar">
          <div className="breadcrumb">
            Workspace
            <ChevronRight size={14} />
            <span>{presentation.title}</span>
          </div>
          <div className="topbar-right">
            <span className="secure-label">
              <span className="status-dot" />
              Secure workspace
            </span>
            <span className="profile" title={currentUser.email}>
              {initials(currentUser.name, currentUser.email)}
            </span>
          </div>
        </header>
        <div className="page">
          <div className="page-title">
            <div>
              <div className="eyebrow">
                {view === 'records' ? 'RELATIONSHIPS' : 'WORKSPACE SETTINGS'}
              </div>
              <h1>{presentation.title}</h1>
              <p className="muted">{presentation.description}</p>
            </div>
            {presentation.actionLabel && (
              <button className="primary" onClick={openPrimaryAction}>
                <Plus size={18} />
                {presentation.actionLabel}
              </button>
            )}
          </div>
          {busy && (
            <p role="status" className="loading-message">
              Updating workspace…
            </p>
          )}
          {actionResult?.ok &&
            actionResult.intent !== 'agent' &&
            actionResult.intent !== 'invite' &&
            !busy && (
              <p role="status" className="success-message">
                {actionMessages[actionResult.intent]}
              </p>
            )}
          {error && (
            <div role="alert" className="error-banner">
              {error}
            </div>
          )}
          {view === 'records' && (
            <RecordsView
              key={section.id}
              section={section}
              records={records}
              canManage={currentUser.role === 'admin'}
              onOpenRecord={openRecord}
              onAddField={() => setModal('field')}
              onDeleteRecord={(record) => {
                setEditing(record);
                setModal('delete');
              }}
            />
          )}
          {view === 'fields' && (
            <FieldsView
              sections={sections}
              section={section}
              onSelectSection={(identifier) => setParams({ view: 'fields', section: identifier })}
              onAddField={() => setModal('field')}
            />
          )}
          {view === 'team' && (
            <TeamView
              users={users}
              invites={invites}
              currentUserId={currentUser.id}
              inviteLink={inviteLink}
              busy={busy}
              onDismissLink={() => setInviteLink('')}
            />
          )}
          {view === 'agents' && (
            <AgentsView
              agents={agents}
              sections={sections}
              token={token}
              busy={busy}
              onDismissToken={() => setToken('')}
              onCreateAgent={() => setModal('agent')}
              onRevokeAgent={(agent) => {
                setRevoke(agent);
                setModal('revoke');
              }}
            />
          )}
        </div>
      </main>
      <WorkspaceDialogs
        modal={modal}
        editing={editing}
        revoke={revoke}
        section={section}
        busy={busy}
        error={error}
        onClose={() => setModal(null)}
      />
    </div>
  );
}
function initials(name: string, email: string) {
  const source = name.trim() || email.split('@')[0] || email;
  const parts = source.split(/[\s._-]+/).filter(Boolean);
  const letters = parts.length > 1 ? `${parts[0][0]}${parts[1][0]}` : source.slice(0, 2);
  return letters.toUpperCase();
}
export function ErrorBoundary() {
  const error = useRouteError();
  return (
    <main className="error-page">
      <h1>Unable to load your workspace.</h1>
      <p>
        {isRouteErrorResponse(error)
          ? error.statusText
          : 'The server could not complete the request. Check that the API and database are running.'}
      </p>
      <a className="primary" href="/">
        Try again
      </a>
    </main>
  );
}
