import * as Tabs from '@radix-ui/react-tabs';
import { useBeforeUnload, useBlocker, useSearchParams } from '@remix-run/react';
import { ArrowLeft } from 'lucide-react';
import { useCallback, useState } from 'react';
import { Modal } from '../../components/modal';
import type { ItemViewDefinition, RecordDetail, Section, Viewer } from '../../types/crm';
import { enabledItemViews } from './registry';

interface Props {
  section: Section;
  record: RecordDetail;
  catalog: ItemViewDefinition[];
  viewer: Viewer;
}

export function ItemDetailScreen({ section, record, catalog, viewer }: Props) {
  const [params, setParams] = useSearchParams();
  const [pendingChanges, setPendingChanges] = useState(false);
  const views = enabledItemViews(section.views, catalog);
  const requested = params.get('tab') || 'info';
  const active = views.find((view) => view.settings.id === requested) || views[0];
  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      pendingChanges &&
      (currentLocation.pathname !== nextLocation.pathname ||
        currentLocation.search !== nextLocation.search),
  );
  useBeforeUnload(
    useCallback(
      (event) => {
        if (!pendingChanges) return;
        event.preventDefault();
        event.returnValue = '';
      },
      [pendingChanges],
    ),
  );
  function changeTab(id: string) {
    if (active.settings.id === id) return;
    const next = new URLSearchParams(params);
    next.set('tab', id);
    setParams(next, { replace: true, preventScrollReset: true });
  }
  function backToSection() {
    const next = new URLSearchParams(params);
    next.delete('record');
    next.delete('tab');
    setParams(next);
  }
  function renderView(view: (typeof views)[number]) {
    const Component = view.plugin.Component;
    return (
      <Component
        section={section}
        record={record}
        viewer={viewer}
        settings={view.settings}
        onPendingChangesChange={setPendingChanges}
      />
    );
  }
  return (
    <div className="item-detail">
      <button type="button" className="item-back" onClick={backToSection}>
        <ArrowLeft size={16} />
        {section.name}
      </button>
      <div className="item-detail-title">
        <div>
          <h1>{String(record.data.name || 'Unnamed record')}</h1>
          <p className="muted">{section.name}</p>
        </div>
        <span className="item-updated">Updated {record.updated_at.slice(0, 10)}</span>
      </div>
      {views.length === 1 ? (
        renderView(active)
      ) : (
        <Tabs.Root value={active.settings.id} onValueChange={changeTab} activationMode="manual">
          <Tabs.List aria-label="Item views" className="item-tabs">
            {views.map((view) => (
              <Tabs.Trigger key={view.settings.id} value={view.settings.id}>
                {view.definition?.label || view.settings.id}
              </Tabs.Trigger>
            ))}
          </Tabs.List>
          {views.map((view) => (
            <Tabs.Content
              key={view.settings.id}
              value={view.settings.id}
              className="item-tab-panel"
            >
              {renderView(view)}
            </Tabs.Content>
          ))}
        </Tabs.Root>
      )}
      {blocker.state === 'blocked' && (
        <Modal
          title="Discard unsaved changes?"
          open
          onOpenChange={(open) => {
            if (!open) blocker.reset();
          }}
        >
          <p>Your changes have not been saved.</p>
          <div className="modal-actions">
            <button className="secondary" onClick={() => blocker.reset()}>
              Keep editing
            </button>
            <button
              className="danger"
              onClick={() => {
                setPendingChanges(false);
                blocker.proceed();
              }}
            >
              Discard changes
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}
