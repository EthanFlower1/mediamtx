import { configureAxe, toHaveNoViolations } from 'jest-axe'
import { expect } from 'vitest'

// Extend vitest expect with axe matchers
expect.extend(toHaveNoViolations)

/**
 * Pre-configured axe instance for WCAG 2.1 AA compliance.
 * Runs against rendered DOM containers in unit tests.
 *
 * - Disables the `region` rule in unit tests because components are
 *   rendered in isolation without the full page landmark structure.
 * - Disables `color-contrast` because JSDOM does not compute styles.
 */
export const axe = configureAxe({
  rules: {
    // Landmarks only make sense in full-page context, not isolated component renders
    region: { enabled: false },
    // JSDOM cannot compute colors so contrast checks produce false positives
    'color-contrast': { enabled: false },
  },
})

export { toHaveNoViolations }
