import { useFetcher } from '@remix-run/react';
import { LockKeyhole, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import type { ItemViewDefinition, Section, SectionView } from '../../types/crm';
import type { workspaceAction } from '../workspace/workspace.server';
import { itemViewPlugins } from '../record-detail/registry';
import { ViewConfigFields } from './view-config-fields';

export function ItemViewsEditor({
  section,
  catalog,
}: {
  section: Section;
  catalog: ItemViewDefinition[];
}) {
  const fetcher = useFetcher<typeof workspaceAction>();
  const [views, setViews] = useState<SectionView[]>(section.views);
  const [selected, setSelected] = useState('');
  const busy = fetcher.state !== 'idle';
  const available = catalog.filter(
    (definition) =>
      !views.some((view) => view.id === definition.id) && itemViewPlugins[definition.id],
  );
  const dirty = JSON.stringify(views) !== JSON.stringify(section.views);
  function update(id: string, changes: Partial<SectionView>) {
    setViews((current) => current.map((view) => (view.id === id ? { ...view, ...changes } : view)));
  }
  function addView() {
    if (!selected) return;
    setViews((current) => [...current, { id: selected, enabled: true, config: {} }]);
    setSelected('');
  }
  return (
    <section className="section-views" aria-label="Item views configuration">
      <div className="panel-heading">
        <div>
          <h2>Item views</h2>
          <p className="muted">
            Choose the views available when opening an item in {section.name}.
          </p>
        </div>
      </div>
      <fetcher.Form method="post">
        <input type="hidden" name="intent" value="item-views" />
        <input type="hidden" name="section" value={section.id} />
        <input type="hidden" name="views" value={JSON.stringify(views)} />
        <div className="configured-views">
          {views.map((view) => {
            const definition = catalog.find((candidate) => candidate.id === view.id);
            const required = view.id === 'info' || definition?.required;
            return (
              <div key={view.id} className="configured-view">
                <div className="configured-view-heading">
                  <div>
                    <h3>{definition?.label || view.id}</h3>
                    <p className="muted">
                      {definition?.description || 'This view is unavailable in this app version.'}
                    </p>
                  </div>
                  <div className="configured-view-actions">
                    {required ? (
                      <span className="required-view">
                        <LockKeyhole size={15} />
                        Always enabled
                      </span>
                    ) : (
                      <>
                        <label className="inline-label">
                          <input
                            type="checkbox"
                            checked={view.enabled}
                            disabled={busy}
                            onChange={(event) => update(view.id, { enabled: event.target.checked })}
                          />
                          Enabled
                        </label>
                        <button
                          type="button"
                          className="icon-button"
                          aria-label={`Remove ${definition?.label || view.id} view`}
                          disabled={busy}
                          onClick={() =>
                            setViews((current) =>
                              current.filter((candidate) => candidate.id !== view.id),
                            )
                          }
                        >
                          <Trash2 size={17} />
                        </button>
                      </>
                    )}
                  </div>
                </div>
                {definition && (
                  <ViewConfigFields
                    fields={definition.config_fields}
                    values={view.config}
                    disabled={busy}
                    onChange={(key, value) =>
                      update(view.id, { config: { ...view.config, [key]: value } })
                    }
                  />
                )}
              </div>
            );
          })}
        </div>
        {available.length > 0 && (
          <div className="add-item-view">
            <label>
              Available views
              <select
                value={selected}
                disabled={busy}
                onChange={(event) => setSelected(event.target.value)}
              >
                <option value="">Select a view</option>
                {available.map((definition) => (
                  <option key={definition.id} value={definition.id}>
                    {definition.label}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              className="secondary"
              disabled={busy || !selected}
              onClick={addView}
            >
              <Plus size={16} />
              Add view
            </button>
          </div>
        )}
        {fetcher.data?.error && (
          <p role="alert" className="error-banner">
            {fetcher.data.error}
          </p>
        )}
        {fetcher.data?.ok && !dirty && (
          <p role="status" className="success-message">
            Item views saved.
          </p>
        )}
        <div className="section-view-save">
          <button type="submit" className="primary" disabled={busy || !dirty}>
            {busy ? 'Saving…' : 'Save views'}
          </button>
        </div>
      </fetcher.Form>
    </section>
  );
}
