// ui/src/theme/typography.ts
//
// String constants for the HUD type ramp helpers. Use these instead of
// stringly-typing class names so refactors stay safe.
//
//   import { hudType } from '~/theme/typography'
//   <span className={hudType.monoSection}>DEVICE INFO</span>

export const hudType = {
  monoLabel: 'text-mono-label',
  monoSection: 'text-mono-section',
  monoData: 'text-mono-data',
  monoDataLg: 'text-mono-data-lg',
  monoTimestamp: 'text-mono-timestamp',
  monoStatus: 'text-mono-status',
  monoControl: 'text-mono-control',
  pageTitle: 'text-page-title',
  cameraName: 'text-camera-name',
  body: 'text-body-hud',
  button: 'text-button-hud',
  alert: 'text-alert-hud',
} as const

export type HudTypeKey = keyof typeof hudType
