import {
  useLoaderData,
  useActionData,
  useLocation,
  useNavigation,
  useSearchParams,
  useRouteError,
  isRouteErrorResponse,
  type Location,
  type Navigation,
} from '@remix-run/react';
import { viewPresentation, actionMessages } from '../features/workspace/presentation';
import { ChevronRight, Plus } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { workspaceLoader, workspaceAction } from '../features/workspace/workspace.server';
import { Sidebar } from '../features/workspace/sidebar';
import { RecordsView } from '../features/workspace/records-view';
import { FieldsView } from '../features/workspace/fields-view';
import { AgentsView } from '../features/workspace/agents-view';
import { TeamView } from '../features/workspace/team-view';
import { WebhooksView } from '../features/workspace/webhooks-view';
import { WorkspaceDialogs } from '../features/workspace/workspace-dialogs';
import { MentionsBell } from '../features/workspace/mentions-bell';
import type { Agent, CRMRecord, ModalKind } from '../types/crm';
export const loader = workspaceLoader;
export const action = workspaceAction;
export default function Workspace() {
  const {
    sections,
    section,
    view,
    page,
    agents,
    users,
    invites,
    webhooks,
    deliveries,
    selectedWebhookId,
    focusRecord,
    currentUser,
  } = useLoaderData<typeof loader>();
  const actionResult = useActionData<typeof action>();
  const navigation = useNavigation();
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const [modal, setModal] = useState<ModalKind | null>(null);
  const [editing, setEditing] = useState<CRMRecord | null>(null);
  const [revoke, setRevoke] = useState<Agent | null>(null);
  const [token, setToken] = useState('');
  const [inviteLink, setInviteLink] = useState('');
  const [webhookEditor, setWebhookEditor] = useState(false);
  const busy = navigation.state !== 'idle' && !isListRefinement(navigation, location);
  const openedFor = useRef('');
  useEffect(() => {
    // One open per navigation: revalidation after a save keeps the key, a new link click changes it.
    if (!focusRecord || openedFor.current === location.key) return;
    openedFor.current = location.key;
    setEditing(focusRecord);
    setModal('record');
  }, [focusRecord, location.key]);
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
    if (view === 'webhooks') {
      setWebhookEditor(true);
      return;
    }
    if (presentation.actionModal === 'record') openRecord(null);
    else setModal(presentation.actionModal);
  }
  function closeModal() {
    setModal(null);
    if (params.has('record')) {
      const next = new URLSearchParams(params);
      next.delete('record');
      setParams(next, { replace: true });
    }
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
            <MentionsBell />
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
              page={page}
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
          {view === 'webhooks' && (
            <WebhooksView
              endpoints={webhooks}
              deliveries={deliveries}
              sections={sections}
              selectedId={selectedWebhookId}
              createOpen={webhookEditor}
              busy={busy}
              onCreateOpenChange={setWebhookEditor}
            />
          )}
        </div>
      </main>
      <WorkspaceDialogs
        modal={modal}
        editing={editing}
        revoke={revoke}
        section={section}
        viewer={{ id: currentUser.id, role: currentUser.role }}
        busy={busy}
        error={error}
        onClose={closeModal}
      />
    </div>
  );
}
const listParams = ['q', 'f', 'sort', 'dir'];
// Searching, filtering and sorting reload the list in place; they should not flash the global banner.
function isListRefinement(navigation: Navigation, location: Location) {
  if (navigation.state !== 'loading' || navigation.formMethod || !navigation.location) return false;
  if (navigation.location.pathname !== location.pathname) return false;
  const next = new URLSearchParams(navigation.location.search);
  const current = new URLSearchParams(location.search);
  for (const name of listParams) {
    next.delete(name);
    current.delete(name);
  }
  return next.toString() === current.toString();
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
