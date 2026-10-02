import { Form, Link, useActionData, useRevalidator, useSearchParams } from '@remix-run/react';
import { Fragment } from 'react';
import { Plus, RefreshCw, Webhook } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Modal } from '../../components/modal';
import type { Section, WebhookDelivery, WebhookEndpoint } from '../../types/crm';
import { webhookEventLabel, webhookEvents } from './presentation';
import type { workspaceAction } from './workspace.server';

interface Props {
  endpoints: WebhookEndpoint[];
  deliveries: WebhookDelivery[];
  sections: Section[];
  selectedId: string;
  createOpen: boolean;
  busy: boolean;
  onCreateOpenChange: (open: boolean) => void;
}

function formatStamp(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('en', {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(date);
}

function sectionLabel(sections: Section[], sectionId: string | null) {
  if (!sectionId) return 'Every section';
  return sections.find((section) => section.id === sectionId)?.name || sectionId;
}

function statusText(endpoint: WebhookEndpoint) {
  if (endpoint.auto_disabled) {
    return `Paused after ${endpoint.consecutive_failures} consecutive failures`;
  }
  if (!endpoint.enabled) return 'Disabled';
  if (endpoint.consecutive_failures > 0) {
    return `${endpoint.consecutive_failures} consecutive failures`;
  }
  return 'Delivering';
}

function statusClass(endpoint: WebhookEndpoint) {
  if (!endpoint.enabled || endpoint.consecutive_failures > 0) return 'webhook-status is-paused';
  return 'webhook-status is-live';
}

function authText(endpoint: WebhookEndpoint) {
  const parts: string[] = [];
  if (endpoint.signing_secret_set) parts.push('HMAC signature');
  if (endpoint.custom_header_set && endpoint.custom_header_name) {
    parts.push(endpoint.custom_header_name);
  }
  if (parts.length === 0) return 'Add a signing secret or a header';
  return parts.join(', ');
}

function deliveryStatus(status: string) {
  if (status === 'succeeded') return 'Delivered';
  if (status === 'failed') return 'Failed';
  if (status === 'inflight') return 'Sending';
  return 'Waiting';
}

export function WebhooksView({
  endpoints,
  deliveries,
  sections,
  selectedId,
  createOpen,
  busy,
  onCreateOpenChange,
}: Props) {
  const actionResult = useActionData<typeof workspaceAction>();
  const revalidator = useRevalidator();
  const [, setParams] = useSearchParams();
  const [editing, setEditing] = useState<WebhookEndpoint | null>(null);
  const [removing, setRemoving] = useState<WebhookEndpoint | null>(null);
  const selected = endpoints.find((endpoint) => endpoint.id === selectedId);
  useEffect(() => {
    if (createOpen) setEditing(null);
  }, [createOpen]);
  useEffect(() => {
    if (!actionResult?.ok) return;
    if (actionResult.intent === 'webhook-create' || actionResult.intent === 'webhook-update') {
      setEditing(null);
      onCreateOpenChange(false);
    }
    if (actionResult.intent === 'webhook-delete') setRemoving(null);
    if (actionResult.intent === 'webhook-test' && actionResult.endpointId) {
      setParams({ view: 'webhooks', endpoint: actionResult.endpointId });
    }
  }, [actionResult, onCreateOpenChange, setParams]);
  const editor = editing || (createOpen ? emptyEndpoint() : null);
  const editorError =
    actionResult &&
    !actionResult.ok &&
    (actionResult.intent === 'webhook-create' || actionResult.intent === 'webhook-update')
      ? actionResult.error
      : undefined;
  const removeError =
    actionResult && !actionResult.ok && actionResult.intent === 'webhook-delete'
      ? actionResult.error
      : undefined;
  return (
    <>
      <div className="agent-intro">
        <Webhook size={24} />
        <div>
          <h2>Events leave with the change that caused them.</h2>
          <p>
            Each endpoint receives the events you subscribe to. A signing secret adds an HMAC
            signature. A custom header can carry a static sender key. Secrets stay on the server
            after you save them.
          </p>
        </div>
      </div>
      {endpoints.length === 0 ? (
        <div className="empty agents-empty">
          <Webhook size={34} />
          <h2>No endpoints yet.</h2>
          <p>Add a public https address, choose the events, and send a test.</p>
          <button className="primary" onClick={() => onCreateOpenChange(true)}>
            <Plus size={17} />
            Add webhook
          </button>
        </div>
      ) : (
        endpoints.map((endpoint) => (
          <Fragment key={endpoint.id}>
            <article
              className={
                endpoint.id === selectedId
                  ? 'agent-panel webhook-card selected'
                  : 'agent-panel webhook-card'
              }
            >
              <div className="panel-heading">
                <h2 className="webhook-url" title={endpoint.url}>
                  {endpoint.url}
                </h2>
                <span className={statusClass(endpoint)}>{statusText(endpoint)}</span>
              </div>
              {endpoint.description ? (
                <p className="webhook-description">{endpoint.description}</p>
              ) : null}
              <dl className="webhook-facts">
                <dt>Events</dt>
                <dd>{endpoint.event_types.map(webhookEventLabel).join(', ')}</dd>
                <dt>Section</dt>
                <dd>{sectionLabel(sections, endpoint.section_id)}</dd>
                <dt>Authentication</dt>
                <dd>{authText(endpoint)}</dd>
              </dl>
              <div className="webhook-actions">
                <Form method="post">
                  <input type="hidden" name="intent" value="webhook-test" />
                  <input type="hidden" name="id" value={endpoint.id} />
                  <button className="secondary" disabled={busy}>
                    Send test event
                  </button>
                </Form>
                <Link className="secondary" to={`/?view=webhooks&endpoint=${endpoint.id}`}>
                  Delivery log
                </Link>
                <button
                  type="button"
                  className="secondary"
                  onClick={() => {
                    onCreateOpenChange(false);
                    setEditing(endpoint);
                  }}
                >
                  Edit
                </button>
                <Form method="post">
                  <input
                    type="hidden"
                    name="intent"
                    value={endpoint.enabled ? 'webhook-disable' : 'webhook-enable'}
                  />
                  <input type="hidden" name="id" value={endpoint.id} />
                  <button className="secondary" disabled={busy}>
                    {endpoint.enabled ? 'Disable' : 'Enable'}
                  </button>
                </Form>
                <button
                  type="button"
                  className="danger-text"
                  onClick={() => setRemoving(endpoint)}
                  disabled={busy}
                >
                  Delete
                </button>
              </div>
            </article>
            {selected && selected.id === endpoint.id && (
              <section className="delivery-log" aria-labelledby="delivery-log-title">
                <div className="panel-heading">
                  <h2 id="delivery-log-title">Delivery log</h2>
                  <button
                    type="button"
                    className="secondary"
                    onClick={() => revalidator.revalidate()}
                    disabled={revalidator.state !== 'idle'}
                  >
                    <RefreshCw size={16} />
                    Refresh
                  </button>
                </div>
                <p className="muted webhook-log-target" title={selected.url}>
                  {selected.url}
                </p>
                {deliveries.length === 0 ? (
                  <p className="team-empty">
                    No deliveries yet. Send a test event to see the first attempt.
                  </p>
                ) : (
                  <div className="delivery-scroll">
                    <table className="delivery-table">
                      <thead>
                        <tr>
                          <th>When</th>
                          <th>Event</th>
                          <th>Status</th>
                          <th className="num">Code</th>
                          <th className="num">Latency</th>
                          <th className="num">Attempts</th>
                          <th>Response</th>
                        </tr>
                      </thead>
                      <tbody>
                        {deliveries.map((delivery) => (
                          <tr key={delivery.id}>
                            <td data-label="When">{formatStamp(delivery.updated_at)}</td>
                            <td data-label="Event">{webhookEventLabel(delivery.event_type)}</td>
                            <td
                              data-label="Status"
                              className={delivery.status === 'failed' ? 'is-failed' : undefined}
                            >
                              {deliveryStatus(delivery.status)}
                            </td>
                            <td className="num" data-label="Code">
                              {delivery.status_code ?? ''}
                            </td>
                            <td className="num" data-label="Latency">
                              {delivery.latency_ms === null ? '' : `${delivery.latency_ms} ms`}
                            </td>
                            <td className="num" data-label="Attempts">
                              {delivery.attempt_count} of {delivery.max_attempts}
                            </td>
                            <td data-label="Response">
                              <p className="delivery-response" title={delivery.response}>
                                {delivery.response}
                              </p>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </section>
            )}
          </Fragment>
        ))
      )}
      {editor && (
        <Modal
          title={editor.id ? 'Edit webhook' : 'Add webhook'}
          open
          onOpenChange={(open) => {
            if (open) return;
            setEditing(null);
            onCreateOpenChange(false);
          }}
        >
          <WebhookForm
            endpoint={editor}
            sections={sections}
            busy={busy}
            error={editorError}
            onCancel={() => {
              setEditing(null);
              onCreateOpenChange(false);
            }}
          />
        </Modal>
      )}
      {removing && (
        <Modal
          title="Delete this webhook?"
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
            <input type="hidden" name="intent" value="webhook-delete" />
            <input type="hidden" name="id" value={removing.id} />
            <p>
              <b className="webhook-url">{removing.url}</b> stops receiving events. Past deliveries
              are removed with it.
            </p>
            <div className="modal-actions">
              <button type="button" className="secondary" onClick={() => setRemoving(null)}>
                Cancel
              </button>
              <button className="danger" disabled={busy}>
                {busy ? 'Deleting…' : 'Delete webhook'}
              </button>
            </div>
          </Form>
        </Modal>
      )}
    </>
  );
}

function emptyEndpoint(): WebhookEndpoint {
  return {
    id: '',
    url: '',
    description: '',
    event_types: ['record.created', 'record.updated', 'record.deleted'],
    section_id: null,
    enabled: true,
    auto_disabled: false,
    consecutive_failures: 0,
    signing_secret_set: false,
    custom_header_name: '',
    custom_header_set: false,
    created_at: '',
    updated_at: '',
    failure_limit: 10,
    max_attempts: 5,
  };
}

function WebhookForm({
  endpoint,
  sections,
  busy,
  error,
  onCancel,
}: {
  endpoint: WebhookEndpoint;
  sections: Section[];
  busy: boolean;
  error?: string;
  onCancel: () => void;
}) {
  const editing = endpoint.id !== '';
  return (
    <Form method="post" className="modal-form" key={`${endpoint.id}:${endpoint.updated_at}`}>
      <input type="hidden" name="intent" value={editing ? 'webhook-update' : 'webhook-create'} />
      {editing ? <input type="hidden" name="id" value={endpoint.id} /> : null}
      <label>
        URL
        <input
          name="url"
          type="url"
          required
          defaultValue={endpoint.url}
          placeholder="https://example.com/hooks/crm"
          autoComplete="off"
        />
      </label>
      <label>
        Description
        <input
          name="description"
          defaultValue={endpoint.description}
          maxLength={200}
          autoComplete="off"
        />
      </label>
      <fieldset className="event-choices">
        <legend>Events</legend>
        <div>
          {webhookEvents.map((event) => (
            <label className="choice" key={event.id}>
              <input
                type="checkbox"
                name={`event:${event.id}`}
                defaultChecked={endpoint.event_types.includes(event.id)}
              />
              {event.label}
            </label>
          ))}
        </div>
      </fieldset>
      <label>
        Section
        <select name="section_id" defaultValue={endpoint.section_id || ''}>
          <option value="">Every section</option>
          {sections.map((section) => (
            <option key={section.id} value={section.id}>
              {section.name}
            </option>
          ))}
        </select>
        <small>Record, field, and timeline events outside this section are skipped.</small>
      </label>
      <label className="choice">
        <input type="checkbox" name="enabled" defaultChecked={endpoint.enabled} />
        Deliver events
      </label>
      <label>
        Signing secret
        <input
          name="signing_secret"
          type="password"
          autoComplete="new-password"
          minLength={editing ? undefined : 8}
          placeholder={endpoint.signing_secret_set ? 'Saved' : 'At least 8 characters'}
        />
        {editing ? (
          <small>Leave blank to keep the saved secret. It is not shown again.</small>
        ) : (
          <small>Used for X-CRM-Signature. It is not shown again.</small>
        )}
      </label>
      {endpoint.signing_secret_set ? (
        <label className="choice">
          <input type="checkbox" name="clear_signing_secret" />
          Remove signing secret
        </label>
      ) : null}
      <label>
        Custom header name
        <input
          name="custom_header_name"
          defaultValue={endpoint.custom_header_name}
          autoComplete="off"
        />
        <small>Example: Authorization</small>
      </label>
      <label>
        Custom header value
        <input
          name="custom_header_value"
          type="password"
          autoComplete="new-password"
          placeholder={endpoint.custom_header_set ? 'Saved' : 'Bearer …'}
        />
        {editing ? (
          <small>Leave blank to keep the saved value.</small>
        ) : (
          <small>Sent exactly as stored, for a static sender key.</small>
        )}
      </label>
      {endpoint.custom_header_set ? (
        <label className="choice">
          <input type="checkbox" name="clear_custom_header" />
          Remove custom header
        </label>
      ) : null}
      {error ? (
        <p role="alert" className="error">
          {error}
        </p>
      ) : null}
      <div className="modal-actions">
        <button type="button" className="secondary" onClick={onCancel}>
          Cancel
        </button>
        <button className="primary" disabled={busy}>
          {busy ? 'Saving…' : 'Save webhook'}
        </button>
      </div>
    </Form>
  );
}
