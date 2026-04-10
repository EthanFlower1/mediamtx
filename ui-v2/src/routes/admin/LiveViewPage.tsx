// KAI-323: Customer Admin Live View page — /admin/live.
//
// Responsibilities:
//   - Camera picker (multi-select from existing cameras API)
//   - Layout preset switcher: 1x1 / 2x2 / 3x3 / 4x4 (native CSS grid)
//   - Stubbed video tiles (<video>) with status indicator + focus/keyboard nav
//   - Empty state when no cameras selected
//   - Enter → navigate to /admin/playback/:cameraId?live=true
//
// This page is a scaffold — real WebRTC streaming lands with KAI-334.
// Status, layout buttons, and markers encode state via icon + text + border,
// never color alone (KAI seam rule).

import { useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';

import {
  camerasQueryKeys,
  listCameras,
  type Camera,
  type CameraListFilters,
} from '@/api/cameras';
import { useSessionStore } from '@/stores/session';

type LayoutPreset = '1x1' | '2x2' | '3x3' | '4x4';

const LAYOUTS: readonly LayoutPreset[] = ['1x1', '2x2', '3x3', '4x4'] as const;

function tilesForLayout(layout: LayoutPreset): number {
  switch (layout) {
    case '1x1':
      return 1;
    case '2x2':
      return 4;
    case '3x3':
      return 9;
    case '4x4':
      return 16;
  }
}

function columnsForLayout(layout: LayoutPreset): number {
  switch (layout) {
    case '1x1':
      return 1;
    case '2x2':
      return 2;
    case '3x3':
      return 3;
    case '4x4':
      return 4;
  }
}

export function LiveViewPage(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);

  const [layout, setLayout] = useState<LayoutPreset>('2x2');
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [focusedTileIndex, setFocusedTileIndex] = useState(0);
  const tileRefs = useRef<Array<HTMLDivElement | null>>([]);

  const filters: CameraListFilters = useMemo(
    () => ({ tenantId }),
    [tenantId],
  );

  const camerasQuery = useQuery<Camera[]>({
    queryKey: camerasQueryKeys.list(tenantId, {}),
    queryFn: () => listCameras(filters),
  });

  const cameras = useMemo(
    () => camerasQuery.data ?? [],
    [camerasQuery.data],
  );
  const cameraById = useMemo(() => {
    const m = new Map<string, Camera>();
    for (const c of cameras) m.set(c.id, c);
    return m;
  }, [cameras]);

  const selectedCameras = useMemo(
    () =>
      selectedIds
        .map((id) => cameraById.get(id))
        .filter((c): c is Camera => c !== undefined),
    [selectedIds, cameraById],
  );

  const tileCount = tilesForLayout(layout);
  const tiles: Array<Camera | null> = useMemo(() => {
    const out: Array<Camera | null> = [];
    for (let i = 0; i < tileCount; i++) {
      out.push(selectedCameras[i] ?? null);
    }
    return out;
  }, [selectedCameras, tileCount]);

  const handleToggleCamera = useCallback((cameraId: string) => {
    setSelectedIds((prev) =>
      prev.includes(cameraId)
        ? prev.filter((id) => id !== cameraId)
        : [...prev, cameraId],
    );
  }, []);

  const handleTileKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>, index: number) => {
      const columns = columnsForLayout(layout);
      let nextIndex = index;
      if (e.key === 'ArrowRight') nextIndex = Math.min(tileCount - 1, index + 1);
      else if (e.key === 'ArrowLeft') nextIndex = Math.max(0, index - 1);
      else if (e.key === 'ArrowDown')
        nextIndex = Math.min(tileCount - 1, index + columns);
      else if (e.key === 'ArrowUp') nextIndex = Math.max(0, index - columns);
      else if (e.key === 'Enter' || e.key === ' ') {
        const tile = tiles[index];
        if (tile) {
          e.preventDefault();
          navigate(`/admin/playback/${tile.id}?live=true`);
        }
        return;
      } else {
        return;
      }
      e.preventDefault();
      setFocusedTileIndex(nextIndex);
      tileRefs.current[nextIndex]?.focus();
    },
    [layout, tileCount, tiles, navigate],
  );

  return (
    <main
      aria-label={t('liveView.pageLabel')}
      data-testid="live-view-page"
      className="live-view-page"
    >
      <nav aria-label={t('liveView.breadcrumbAriaLabel')}>
        <ol>
          <li>{tenantName}</li>
          <li aria-current="page">{t('liveView.title')}</li>
        </ol>
      </nav>

      <header>
        <h1>{t('liveView.title')}</h1>
      </header>

      {/* Layout switcher */}
      <section aria-label={t('liveView.layout.sectionLabel')}>
        <div
          role="radiogroup"
          aria-label={t('liveView.layout.sectionLabel')}
          data-testid="live-view-layout-switcher"
        >
          {LAYOUTS.map((preset) => (
            <button
              key={preset}
              type="button"
              role="radio"
              aria-checked={layout === preset}
              onClick={() => setLayout(preset)}
              data-testid={`live-view-layout-${preset}`}
              data-selected={layout === preset ? 'true' : 'false'}
            >
              {t(`liveView.layout.${preset}`)}
            </button>
          ))}
        </div>
      </section>

      {/* Camera picker */}
      <section aria-label={t('liveView.picker.sectionLabel')}>
        <h2>{t('liveView.picker.heading')}</h2>
        {camerasQuery.isLoading && (
          <p role="status" aria-live="polite">{t('liveView.picker.loading')}</p>
        )}
        {camerasQuery.isError && (
          <p role="alert">{t('liveView.picker.error')}</p>
        )}
        {camerasQuery.isSuccess && (
          <ul data-testid="live-view-camera-picker">
            {cameras.map((camera) => {
              const checked = selectedIds.includes(camera.id);
              return (
                <li key={camera.id}>
                  <label>
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={() => handleToggleCamera(camera.id)}
                      data-testid={`live-view-picker-${camera.id}`}
                      aria-label={t('liveView.picker.toggleAriaLabel', {
                        name: camera.name,
                      })}
                    />
                    <span>{camera.name}</span>
                  </label>
                </li>
              );
            })}
          </ul>
        )}
      </section>

      {/* Grid */}
      <section
        aria-label={t('liveView.grid.sectionLabel')}
        data-testid="live-view-grid-section"
      >
        {selectedCameras.length === 0 ? (
          <div role="status" data-testid="live-view-empty-state">
            <h2>{t('liveView.emptyTitle')}</h2>
            <p>{t('liveView.emptyBody')}</p>
            <p>{t('liveView.selectCameras')}</p>
          </div>
        ) : (
          <div
            role="grid"
            aria-label={t('liveView.grid.ariaLabel')}
            data-testid="live-view-grid"
            data-layout={layout}
            data-tile-count={tileCount}
            style={{
              display: 'grid',
              gridTemplateColumns: `repeat(${columnsForLayout(layout)}, minmax(0, 1fr))`,
              gap: '0.5rem',
            }}
          >
            {tiles.map((tile, index) => {
              const key = tile ? `tile-${tile.id}` : `tile-empty-${index}`;
              const isOnline = tile?.status === 'online';
              const statusText = tile
                ? t(`liveView.status.${tile.status}`)
                : t('liveView.status.empty');
              const ariaLabel = tile
                ? t('liveView.tile.ariaLabel', {
                    name: tile.name,
                    status: statusText,
                  })
                : t('liveView.tile.emptyAriaLabel', { index: index + 1 });
              return (
                <div
                  key={key}
                  ref={(el) => {
                    tileRefs.current[index] = el;
                  }}
                  role={tile ? 'button' : 'gridcell'}
                  tabIndex={index === focusedTileIndex ? 0 : -1}
                  aria-label={ariaLabel}
                  data-testid={`live-view-tile-${index}`}
                  data-status={tile ? tile.status : 'empty'}
                  onFocus={() => setFocusedTileIndex(index)}
                  onKeyDown={(e) => handleTileKeyDown(e, index)}
                  onClick={() => {
                    if (tile) navigate(`/admin/playback/${tile.id}?live=true`);
                  }}
                  style={{
                    border: tile
                      ? isOnline
                        ? '2px solid currentColor'
                        : '2px dashed currentColor'
                      : '1px dotted currentColor',
                    padding: '0.5rem',
                    minHeight: '120px',
                    outline: 'none',
                  }}
                >
                  {tile ? (
                    <>
                      <div data-testid={`live-view-tile-name-${index}`}>
                        {tile.name}
                      </div>
                      <div
                        data-testid={`live-view-tile-status-${index}`}
                        data-status={tile.status}
                      >
                        {/* Icon + text + border — never color alone (KAI a11y rule). */}
                        <span aria-hidden="true">{isOnline ? '●' : '○'}</span>
                        <span> {statusText}</span>
                      </div>
                      <video
                        data-testid={`live-view-tile-video-${index}`}
                        muted
                        playsInline
                        poster={tile.id}
                        aria-label={t('liveView.tile.videoAriaLabel', {
                          name: tile.name,
                        })}
                      />
                    </>
                  ) : (
                    <div>{statusText}</div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </section>
    </main>
  );
}

export default LiveViewPage;
