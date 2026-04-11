import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import ToastContainer from './Toast'
import { axe } from '../test/axe-helper'

describe('ToastContainer accessibility', () => {
  it('has no axe violations (empty state)', async () => {
    const { container } = render(<ToastContainer />)
    const results = await axe(container)
    expect(results).toHaveNoViolations()
  })

  it('has aria-live region for screen reader announcements', () => {
    render(<ToastContainer />)
    const region = screen.getByRole('status')
    expect(region).toHaveAttribute('aria-live', 'polite')
    expect(region).toHaveAttribute('aria-label', 'Notifications')
  })
})
