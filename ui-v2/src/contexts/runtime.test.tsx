import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';
import { RuntimeContextProvider, useRuntimeContext } from './runtime';
import { i18n } from '@/i18n';
import { runAxe } from '@/test/setup';

function Probe(): JSX.Element {
  const ctx = useRuntimeContext();
  return <div data-testid="kind">{ctx.kind}</div>;
}

function renderAt(path: string) {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[path]}>
        <RuntimeContextProvider>
          <Routes>
            <Route path="*" element={<Probe />} />
          </Routes>
        </RuntimeContextProvider>
      </MemoryRouter>
    </I18nextProvider>,
  );
}

describe('RuntimeContextProvider', () => {
  it('detects admin context from /admin/* path', () => {
    renderAt('/admin/dashboard');
    expect(screen.getByTestId('kind').textContent).toBe('admin');
  });

  it('detects command context from /command/* path', () => {
    renderAt('/command/customers');
    expect(screen.getByTestId('kind').textContent).toBe('command');
  });

  it('falls back to unknown for other paths', () => {
    renderAt('/random');
    expect(screen.getByTestId('kind').textContent).toBe('unknown');
  });

  it('produces zero critical axe violations on the smoke render', async () => {
    const { container } = renderAt('/admin');
    const violations = await runAxe(container);
    const blocking = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(blocking).toEqual([]);
  });
});
