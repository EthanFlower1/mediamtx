// ui/src/components/hud/CornerBrackets.tsx
import { type ReactNode } from 'react'

export interface CornerBracketsProps {
  children: ReactNode
  size?: 'sm' | 'md' | 'lg'
  color?: 'accent' | 'border' | 'success' | 'danger' | 'warning'
  padding?: number
  strokeWidth?: number
}

/**
 * Mirrors clients/flutter/lib/widgets/hud/corner_brackets.dart.
 *
 * L-shaped brackets at each corner of the wrapped child. The brackets are
 * positioned absolutely, so they don't add layout space. The child should
 * be a sized element (image, video, etc.) for best effect.
 */

const sizeMap = { sm: 12, md: 16, lg: 24 }

const colorMap: Record<NonNullable<CornerBracketsProps['color']>, string> = {
  accent: 'text-accent',
  border: 'text-border',
  success: 'text-success',
  danger: 'text-danger',
  warning: 'text-warning',
}

export function CornerBrackets({
  children,
  size = 'md',
  color = 'accent',
  padding = 6,
  strokeWidth = 2,
}: CornerBracketsProps) {
  const px = sizeMap[size]
  const colorClass = colorMap[color]

  return (
    <div className="relative">
      {children}
      <Bracket corner="tl" px={px} stroke={strokeWidth} pad={padding} colorClass={colorClass} />
      <Bracket corner="tr" px={px} stroke={strokeWidth} pad={padding} colorClass={colorClass} />
      <Bracket corner="bl" px={px} stroke={strokeWidth} pad={padding} colorClass={colorClass} />
      <Bracket corner="br" px={px} stroke={strokeWidth} pad={padding} colorClass={colorClass} />
    </div>
  )
}

type Corner = 'tl' | 'tr' | 'bl' | 'br'

function Bracket({
  corner,
  px,
  stroke,
  pad,
  colorClass,
}: {
  corner: Corner
  px: number
  stroke: number
  pad: number
  colorClass: string
}) {
  const positionStyle: React.CSSProperties = {
    width: px,
    height: px,
    position: 'absolute',
    pointerEvents: 'none',
    opacity: 0.4,
  }
  if (corner === 'tl') {
    positionStyle.top = pad
    positionStyle.left = pad
  } else if (corner === 'tr') {
    positionStyle.top = pad
    positionStyle.right = pad
  } else if (corner === 'bl') {
    positionStyle.bottom = pad
    positionStyle.left = pad
  } else {
    positionStyle.bottom = pad
    positionStyle.right = pad
  }

  // The path describes an L-shape oriented for the given corner.
  // tl: down then right (anchor at top-left)
  // tr: left then down (anchor at top-right)
  // bl: up then right
  // br: left then up
  let d = ''
  if (corner === 'tl') d = `M 0 ${px} L 0 0 L ${px} 0`
  else if (corner === 'tr') d = `M 0 0 L ${px} 0 L ${px} ${px}`
  else if (corner === 'bl') d = `M 0 0 L 0 ${px} L ${px} ${px}`
  else d = `M ${px} 0 L ${px} ${px} L 0 ${px}`

  return (
    <svg
      className={colorClass}
      style={positionStyle}
      viewBox={`0 0 ${px} ${px}`}
      fill="none"
      stroke="currentColor"
      strokeWidth={stroke}
      strokeLinecap="square"
      aria-hidden="true"
    >
      <path d={d} />
    </svg>
  )
}
