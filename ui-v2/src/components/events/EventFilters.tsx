import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import type { EventSeverity, EventType } from '@/api/events';

// KAI-324: Filter bar for the Events page.
//
// Four filter dimensions: camera, type, severity, time range.
// Each dimension is an independent controlled input; the parent
// page owns filter state and passes it back down so TanStack
// Query keys stay pure.

export interface EventFiltersState {
  cameraIds: readonly string[];
  types: readonly EventType[];
  severities: readonly EventSeverity[];
  fromIso: string;
  toIso: string;
}

export interface EventFiltersProps {
  readonly cameras: readonly { id: string; name: string }[];
  readonly value: EventFiltersState;
  readonly onChange: (next: EventFiltersState) => void;
  readonly onReset: () => void;
}

const ALL_TYPES: readonly EventType[] = [
  'person.detected',
  'vehicle.detected',
  'package.detected',
  'face.matched',
  'lpr.plate',
  'anomaly.behavioral',
  'line.crossed',
  'intrusion.zone',
];

const ALL_SEVERITIES: readonly EventSeverity[] = [
  'info',
  'low',
  'medium',
  'high',
  'critical',
];

export function EventFilters({
  cameras,
  value,
  onChange,
  onReset,
}: EventFiltersProps): JSX.Element {
  const { t } = useTranslation();

  const toggleCamera = useCallback(
    (id: string): void => {
      const set = new Set(value.cameraIds);
      if (set.has(id)) {
        set.delete(id);
      } else {
        set.add(id);
      }
      onChange({ ...value, cameraIds: Array.from(set) });
    },
    [value, onChange],
  );

  const toggleType = useCallback(
    (type: EventType): void => {
      const set = new Set(value.types);
      if (set.has(type)) {
        set.delete(type);
      } else {
        set.add(type);
      }
      onChange({ ...value, types: Array.from(set) });
    },
    [value, onChange],
  );

  const toggleSeverity = useCallback(
    (severity: EventSeverity): void => {
      const set = new Set(value.severities);
      if (set.has(severity)) {
        set.delete(severity);
      } else {
        set.add(severity);
      }
      onChange({ ...value, severities: Array.from(set) });
    },
    [value, onChange],
  );

  return (
    <section
      aria-label={t('events.filters.sectionLabel')}
      data-testid="events-filters"
      className="flex flex-wrap gap-4 rounded-lg border border-slate-200 bg-white p-4 shadow-sm"
    >
      <fieldset className="min-w-[14rem]">
        <legend className="text-xs font-semibold text-slate-700">
          {t('events.filters.cameras')}
        </legend>
        <div className="mt-1 flex flex-wrap gap-1">
          {cameras.map((cam) => {
            const checked = value.cameraIds.includes(cam.id);
            return (
              <label
                key={cam.id}
                className="inline-flex items-center gap-1 rounded border border-slate-300 bg-white px-2 py-1 text-xs"
              >
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={() => toggleCamera(cam.id)}
                  aria-label={t('events.filters.toggleCameraAriaLabel', {
                    camera: cam.name,
                  })}
                  data-testid={`events-filter-camera-${cam.id}`}
                />
                <span>{cam.name}</span>
              </label>
            );
          })}
        </div>
      </fieldset>

      <fieldset className="min-w-[14rem]">
        <legend className="text-xs font-semibold text-slate-700">
          {t('events.filters.types')}
        </legend>
        <div className="mt-1 flex flex-wrap gap-1">
          {ALL_TYPES.map((type) => {
            const checked = value.types.includes(type);
            return (
              <label
                key={type}
                className="inline-flex items-center gap-1 rounded border border-slate-300 bg-white px-2 py-1 text-xs"
              >
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={() => toggleType(type)}
                  data-testid={`events-filter-type-${type}`}
                />
                <span>{t(`events.type.${type}`)}</span>
              </label>
            );
          })}
        </div>
      </fieldset>

      <fieldset className="min-w-[12rem]">
        <legend className="text-xs font-semibold text-slate-700">
          {t('events.filters.severities')}
        </legend>
        <div className="mt-1 flex flex-wrap gap-1">
          {ALL_SEVERITIES.map((severity) => {
            const checked = value.severities.includes(severity);
            return (
              <label
                key={severity}
                className="inline-flex items-center gap-1 rounded border border-slate-300 bg-white px-2 py-1 text-xs"
              >
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={() => toggleSeverity(severity)}
                  data-testid={`events-filter-severity-${severity}`}
                />
                <span>{t(`events.severity.${severity}`)}</span>
              </label>
            );
          })}
        </div>
      </fieldset>

      <fieldset className="min-w-[14rem]">
        <legend className="text-xs font-semibold text-slate-700">
          {t('events.filters.timeRange')}
        </legend>
        <div className="mt-1 flex flex-col gap-1">
          <label className="text-xs text-slate-600">
            {t('events.filters.from')}
            <input
              type="datetime-local"
              value={value.fromIso}
              onChange={(e) => onChange({ ...value, fromIso: e.target.value })}
              data-testid="events-filter-from"
              className="ml-1 rounded border border-slate-300 px-1 py-0.5 text-xs"
            />
          </label>
          <label className="text-xs text-slate-600">
            {t('events.filters.to')}
            <input
              type="datetime-local"
              value={value.toIso}
              onChange={(e) => onChange({ ...value, toIso: e.target.value })}
              data-testid="events-filter-to"
              className="ml-1 rounded border border-slate-300 px-1 py-0.5 text-xs"
            />
          </label>
        </div>
      </fieldset>

      <div className="flex items-end">
        <button
          type="button"
          onClick={onReset}
          data-testid="events-filters-reset"
          className="rounded border border-slate-300 bg-white px-3 py-1 text-xs text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          {t('events.filters.reset')}
        </button>
      </div>
    </section>
  );
}
