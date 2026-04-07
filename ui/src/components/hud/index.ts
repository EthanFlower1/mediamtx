// ui/src/components/hud/index.ts
//
// Barrel re-export for the HUD component library. Consumers should import
// from this file rather than reaching into individual files:
//
//   import { HudButton, StatusBadge, SectionCard } from '~/components/hud'

export { HudButton, type HudButtonVariant, type HudButtonProps } from './HudButton'
export { HudToggle, type HudToggleProps } from './HudToggle'
export { HudInput, type HudInputProps } from './HudInput'
export { HudTextarea, type HudTextareaProps } from './HudTextarea'
export { HudSelect, type HudSelectOption, type HudSelectProps } from './HudSelect'
export { AnalogSlider, type AnalogSliderProps } from './AnalogSlider'
export {
  SegmentedControl,
  type SegmentedControlOption,
  type SegmentedControlProps,
} from './SegmentedControl'
export { StatusBadge, type StatusVariant, type StatusBadgeProps } from './StatusBadge'
export { CornerBrackets, type CornerBracketsProps } from './CornerBrackets'
export { SectionCard, type SectionCardProps } from './SectionCard'
export { KvRow, type KvRowProps } from './KvRow'
