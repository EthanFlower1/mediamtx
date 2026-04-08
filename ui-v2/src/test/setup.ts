import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';
import axe from 'axe-core';

// KAI-307: Vitest test setup.
//
// - jest-dom matchers for accessible assertions
// - cleanup() between tests
// - axe-core is wired here so any test can call `runAxe(container)`
//   and assert zero critical/serious violations. The CI gate (KAI
//   level rule) is "zero critical or serious axe violations".

afterEach(() => {
  cleanup();
});

export interface AxeViolation {
  id: string;
  impact: string | null | undefined;
  description: string;
  nodes: number;
}

export async function runAxe(node: Element | Document = document): Promise<AxeViolation[]> {
  const results = await axe.run(node, {
    resultTypes: ['violations'],
  });
  return results.violations.map((v) => ({
    id: v.id,
    impact: v.impact,
    description: v.description,
    nodes: v.nodes.length,
  }));
}
