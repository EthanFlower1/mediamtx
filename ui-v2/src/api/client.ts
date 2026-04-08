// KAI-307: Connect-Go client stub.
//
// Per spec, all API calls flow through generated Connect-Go clients
// produced from the shared `.proto` files. Generated code does not
// exist yet (added in KAI-310 / proto pipeline). This file provides
// a typed placeholder factory so feature work can import a stable
// surface that will resolve to real generated services later.
//
// When the generated code lands, replace `createPlaceholderClient`
// with `createPromiseClient(ServiceDefinition, transport)` and
// re-export per-service hooks.

import { createConnectTransport } from '@connectrpc/connect-web';

export const API_BASE_URL = '/api/v1';

export const transport = createConnectTransport({
  baseUrl: API_BASE_URL,
  // Connect protocol; switch to gRPC-Web when on-prem cluster is ready.
  useBinaryFormat: false,
});

// Placeholder service shape — replace with generated PromiseClient<T>.
export interface PlaceholderClient {
  readonly baseUrl: string;
  readonly transport: typeof transport;
}

export function createApiClient(): PlaceholderClient {
  return {
    baseUrl: API_BASE_URL,
    transport,
  };
}

export const apiClient = createApiClient();
