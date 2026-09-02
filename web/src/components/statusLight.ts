export type StatusLightTone = 'success' | 'warning' | 'danger' | 'neutral'
export type StatusLightSize = 'sm' | 'md'

export function statusLightToneClass(tone: StatusLightTone) {
  switch (tone) {
    case 'success':
      return 'bg-[var(--ui-success)]'
    case 'warning':
      return 'bg-[var(--ui-warning)]'
    case 'danger':
      return 'bg-[var(--ui-danger)]'
    case 'neutral':
      return 'bg-[var(--ui-text-muted)]'
  }
}

export function statusLightSizeClass(size: StatusLightSize) {
  return size === 'md' ? 'w-1.5 h-1.5' : 'w-2 h-2'
}

export function statusLightAnimatedClass(animated = true) {
  return animated ? 'animate-pulse' : ''
}
