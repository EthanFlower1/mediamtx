// KAI-323: Customer Admin Playback page — /admin/playback/:eventId.
//
// Responsibilities:
//   - Header + breadcrumb back to /admin/events
//   - Stubbed video player (<video>) — real WebRTC/HLS/RTSP lands KAI-334
//   - PlaybackTimeline (24h scrubber) with AI event markers
//   - Export clip button (calls exportClip stub)
//   - ?live=true → show LIVE badge instead of timeline
//
// All strings through t(). Tenant-scoped query keys. Severity/status
// encoded via icon + text + border (never color alone).

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Link, useParams, useSearchParams } from 'react-router-dom';

import {
  clampSeek,
  exportClip,
  formatClockSec,
  getPlaybackEvent,
  listMarkersForWindow,
  playbackQueryKeys,
  type PlaybackEvent,
  type PlaybackMarker,
} from '@/api/playback';
import { useSessionStore } from '@/stores/session';

const DEFAULT_EXPORT_DURATION_SEC = 60;

interface PlaybackTimelineProps {
  event: PlaybackEvent;
  markers: PlaybackMarker[];
  cursorSec: number;
  onSeek: (atSec: number) => void;
}

function PlaybackTimeline({
  event,
  markers,
  cursorSec,
  onSeek,
}: PlaybackTimelineProps): JSX.Element {
  const { t } = useTranslation();
  const duration = event.windowDurationSec;

  const handleClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      const rect = e.currentTarget.getBoundingClientRect();
      if (rect.width <= 0) return;
      const ratio = (e.clientX - rect.left) / rect.width;
      onSeek(clampSeek(ratio * duration, duration));
    },
    [duration, onSeek],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const step = 60; // 1 min
      if (e.key === 'ArrowRight') {
        e.preventDefault();
        onSeek(clampSeek(cursorSec + step, duration));
      } else if (e.key === 'ArrowLeft') {
        e.preventDefault();
        onSeek(clampSeek(cursorSec - step, duration));
      }
    },
    [cursorSec, duration, onSeek],
  );

  // 24 hourly ticks.
  const ticks = useMemo(() => {
    const arr: number[] = [];
    for (let h = 0; h <= 24; h++) arr.push(h * 3600);
    return arr;
  }, []);

  return (
    <div
      role="slider"
      tabIndex={0}
      aria-label={t('playback.timeline.ariaLabel')}
      aria-valuemin={0}
      aria-valuemax={duration}
      aria-valuenow={Math.floor(cursorSec)}
      aria-valuetext={formatClockSec(cursorSec)}
      data-testid="playback-timeline"
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      style={{
        position: 'relative',
        width: '100%',
        height: '56px',
        border: '2px solid currentColor',
        padding: '0.25rem',
        cursor: 'pointer',
        userSelect: 'none',
      }}
    >
      {/* Hour ticks */}
      {ticks.map((sec) => {
        const pct = (sec / duration) * 100;
        return (
          <span
            key={`tick-${sec}`}
            aria-hidden="true"
            data-testid={`playback-timeline-tick-${sec}`}
            style={{
              position: 'absolute',
              left: `${pct}%`,
              top: 0,
              bottom: '50%',
              width: '1px',
              borderLeft: '1px solid currentColor',
            }}
          />
        );
      })}

      {/* AI markers */}
      {markers.map((marker) => {
        const pct = (marker.atSec / duration) * 100;
        const severityLabel = t(`playback.severity.${marker.severity}`);
        return (
          <span
            key={marker.id}
            role="img"
            data-testid={`playback-marker-${marker.id}`}
            data-severity={marker.severity}
            aria-label={t('playback.marker.ariaLabel', {
              kind: t(`playback.marker.kind.${marker.kind}`),
              at: formatClockSec(marker.atSec),
              severity: severityLabel,
            })}
            title={t('playback.marker.tooltip', {
              kind: t(`playback.marker.kind.${marker.kind}`),
              at: formatClockSec(marker.atSec),
              severity: severityLabel,
            })}
            style={{
              position: 'absolute',
              left: `${pct}%`,
              top: '40%',
              transform: 'translate(-50%, -50%)',
              width: '12px',
              height: '12px',
              border:
                marker.severity === 'critical'
                  ? '2px solid currentColor'
                  : marker.severity === 'warning'
                    ? '2px dashed currentColor'
                    : '2px dotted currentColor',
              // Icon + border + text (aria-label), never color alone.
            }}
          >
            <span aria-hidden="true" style={{ fontSize: '10px' }}>
              {marker.severity === 'critical'
                ? '!'
                : marker.severity === 'warning'
                  ? '*'
                  : '·'}
            </span>
          </span>
        );
      })}

      {/* Cursor */}
      <span
        aria-hidden="true"
        data-testid="playback-timeline-cursor"
        style={{
          position: 'absolute',
          left: `${(cursorSec / duration) * 100}%`,
          top: 0,
          bottom: 0,
          width: '2px',
          background: 'currentColor',
        }}
      />
    </div>
  );
}

export function PlaybackPage(): JSX.Element {
  const { t } = useTranslation();
  const { eventId = '' } = useParams<{ eventId: string }>();
  const [searchParams] = useSearchParams();
  const isLive = searchParams.get('live') === 'true';
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);

  const [cursorSec, setCursorSec] = useState(0);

  const eventQuery = useQuery<PlaybackEvent>({
    queryKey: playbackQueryKeys.event(tenantId, eventId),
    queryFn: () => getPlaybackEvent(tenantId, eventId),
    enabled: eventId.length > 0,
  });

  const event = eventQuery.data;
  const duration = event?.windowDurationSec ?? 0;

  const markersQuery = useQuery<PlaybackMarker[]>({
    queryKey: playbackQueryKeys.markers(tenantId, eventId, 0, duration),
    queryFn: () =>
      listMarkersForWindow({ tenantId, eventId, startSec: 0, endSec: duration }),
    enabled: !isLive && !!event,
  });

  const exportMutation = useMutation({
    mutationFn: () =>
      exportClip({
        tenantId,
        eventId,
        startSec: cursorSec,
        durationSec: DEFAULT_EXPORT_DURATION_SEC,
      }),
  });

  const handleSeek = useCallback(
    (atSec: number) => {
      setCursorSec(clampSeek(atSec, duration));
    },
    [duration],
  );

  // Initialize cursor to event seed once event lands.
  const [cursorInitialized, setCursorInitialized] = useState(false);
  useEffect(() => {
    if (event && !cursorInitialized) {
      setCursorSec(event.seedAtSec);
      setCursorInitialized(true);
    }
  }, [event, cursorInitialized]);

  const markers = useMemo(() => markersQuery.data ?? [], [markersQuery.data]);

  return (
    <main
      aria-label={t('playback.pageLabel')}
      data-testid="playback-page"
      className="playback-page"
    >
      <nav aria-label={t('playback.breadcrumbAriaLabel')}>
        <ol>
          <li>{tenantName}</li>
          <li>
            <Link to="/admin/events">{t('playback.breadcrumb.events')}</Link>
          </li>
          <li aria-current="page">{t('playback.title')}</li>
        </ol>
      </nav>

      <header>
        <h1>{t('playback.title')}</h1>
        {isLive && (
          <span
            role="status"
            data-testid="playback-live-badge"
            aria-label={t('playback.liveLabel')}
            style={{
              display: 'inline-block',
              border: '2px solid currentColor',
              padding: '0.125rem 0.5rem',
              marginInlineStart: '0.5rem',
            }}
          >
            <span aria-hidden="true">● </span>
            {t('playback.liveLabel')}
          </span>
        )}
      </header>

      {eventQuery.isLoading && (
        <p role="status" aria-live="polite">{t('playback.loading')}</p>
      )}
      {eventQuery.isError && (
        <p role="alert">{t('playback.loadError')}</p>
      )}

      {event && (
        <>
          <section aria-label={t('playback.player.sectionLabel')}>
            <video
              data-testid="playback-video"
              controls
              muted
              playsInline
              aria-label={t('playback.player.ariaLabel', {
                name: event.cameraName,
              })}
            />
          </section>

          {isLive ? (
            <p data-testid="playback-live-notice">{t('playback.liveNotice')}</p>
          ) : (
            <section aria-label={t('playback.timeline.sectionLabel')}>
              <PlaybackTimeline
                event={event}
                markers={markers}
                cursorSec={cursorSec}
                onSeek={handleSeek}
              />
              <p data-testid="playback-cursor-label">
                {t('playback.cursorLabel', { at: formatClockSec(cursorSec) })}
              </p>
            </section>
          )}

          <section aria-label={t('playback.actions.sectionLabel')}>
            <button
              type="button"
              data-testid="playback-export-button"
              onClick={() => exportMutation.mutate()}
              disabled={exportMutation.isPending}
              aria-disabled={exportMutation.isPending}
            >
              {exportMutation.isPending
                ? t('playback.exporting')
                : t('playback.exportClip')}
            </button>
            {exportMutation.isSuccess && (
              <p role="status" data-testid="playback-export-success">
                {t('playback.exportSuccess')}
              </p>
            )}
            {exportMutation.isError && (
              <p role="alert" data-testid="playback-export-error">
                {t('playback.exportError')}
              </p>
            )}
          </section>
        </>
      )}
    </main>
  );
}

export default PlaybackPage;
