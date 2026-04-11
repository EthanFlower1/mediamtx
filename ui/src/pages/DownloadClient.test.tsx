import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import DownloadClient from './DownloadClient'
import { axe } from '../test/axe-helper'
import { renderWithRouter } from '../test/render-helper'

describe('DownloadClient page accessibility', () => {
  it('has no axe violations', async () => {
    const { container } = renderWithRouter(<DownloadClient />)
    const results = await axe(container)
    expect(results).toHaveNoViolations()
  })

  it('has a heading hierarchy', () => {
    renderWithRouter(<DownloadClient />)
    const h1 = screen.getByRole('heading', { level: 1 })
    expect(h1).toHaveTextContent(/download/i)
    const h2 = screen.getByRole('heading', { level: 2 })
    expect(h2).toHaveTextContent(/quick setup/i)
  })

  it('download links open in new tab with rel attributes', () => {
    renderWithRouter(<DownloadClient />)
    const links = screen.getAllByRole('link')
    links.forEach((link) => {
      if (link.getAttribute('target') === '_blank') {
        expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'))
      }
    })
  })

  it('has an ordered list for setup steps', () => {
    renderWithRouter(<DownloadClient />)
    const list = screen.getByRole('list')
    expect(list).toBeInTheDocument()
    const items = screen.getAllByRole('listitem')
    expect(items.length).toBeGreaterThanOrEqual(4)
  })
})
