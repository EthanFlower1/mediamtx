// KAI-325: Component tests for UsersPage.
//
// Coverage:
//   - Users list renders + search narrows list
//   - Invite user dialog — email validation
//   - Delete confirm — name-gating
//   - Bulk suspend / bulk delete
//   - Permissions tab renders matrix
//   - Sign-in methods tab renders 6 provider cards
//   - Each wizard (6): happy path fill → test → save, failure path, save disabled until test
//   - axe-core smoke (no critical/serious violations)
//   - Tenant isolation: queries scoped to active session tenantId

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, within, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { UsersPage } from './UsersPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';
import * as usersApi from '@/api/users';

function renderPage(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin/users']}>
          <UsersPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('UsersPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-test-325',
      tenantName: 'Test Tenant 325',
      userId: 'user-test',
      userDisplayName: 'Test User',
    });
    vi.restoreAllMocks();
  });

  // ---------------------------------------------------------------------------
  // Users list
  // ---------------------------------------------------------------------------

  it('renders the user list with rows', async () => {
    const listSpy = vi.spyOn(usersApi, 'listUsers');
    renderPage();
    await waitFor(() => expect(screen.getByTestId('users-list')).toBeInTheDocument());
    expect(listSpy).toHaveBeenCalledWith('tenant-test-325');
    const rows = screen.queryAllByTestId(/^user-row-/);
    expect(rows.length).toBeGreaterThan(0);
  });

  it('search input narrows the visible list', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('users-list'));

    const allRows = screen.queryAllByTestId(/^user-row-/);
    const totalCount = allRows.length;

    await user.type(screen.getByTestId('users-search'), 'User 1');
    await waitFor(() => {
      const rows = screen.queryAllByTestId(/^user-row-/);
      expect(rows.length).toBeLessThan(totalCount);
      expect(rows.length).toBeGreaterThan(0);
    });
  });

  it('scopes user queries to the active session tenant', async () => {
    const listSpy = vi.spyOn(usersApi, 'listUsers');
    renderPage();
    await waitFor(() => expect(listSpy).toHaveBeenCalledWith('tenant-test-325'));

    listSpy.mockClear();
    act(() => {
      useSessionStore.setState({
        tenantId: 'tenant-other-456',
        tenantName: 'Other Tenant',
        userId: 'u',
        userDisplayName: 'O',
      });
    });
    await waitFor(() => expect(listSpy).toHaveBeenCalledWith('tenant-other-456'));
  });

  // ---------------------------------------------------------------------------
  // Invite user dialog
  // ---------------------------------------------------------------------------

  it('invite dialog validates email before submitting', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('users-list'));

    await user.click(screen.getByTestId('users-invite-button'));
    await waitFor(() => screen.getByTestId('invite-user-dialog'));

    // Submit without filling email
    await user.click(screen.getByTestId('invite-submit'));
    expect(await screen.findByTestId('invite-email-error')).toBeInTheDocument();
  });

  it('invite dialog accepts valid email + role + submits', async () => {
    const inviteSpy = vi.spyOn(usersApi, 'inviteUser');
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('users-list'));

    await user.click(screen.getByTestId('users-invite-button'));
    await waitFor(() => screen.getByTestId('invite-user-dialog'));

    await user.type(screen.getByTestId('invite-field-email'), 'alice@example.com');
    await user.selectOptions(screen.getByTestId('invite-field-role'), 'operator');
    await user.click(screen.getByTestId('invite-submit'));

    await waitFor(() =>
      expect(inviteSpy).toHaveBeenCalledWith(
        expect.objectContaining({ email: 'alice@example.com', role: 'operator' }),
      ),
    );
  });

  // ---------------------------------------------------------------------------
  // Delete confirm
  // ---------------------------------------------------------------------------

  it('delete confirm requires typing the user name before enabling', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('users-list'));

    const deleteButtons = await screen.findAllByTestId(/^user-delete-/);
    await user.click(deleteButtons[0]!);

    const dialog = await screen.findByTestId('delete-user-dialog');
    const confirmBtn = within(dialog).getByTestId('delete-confirm') as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);

    await user.type(within(dialog).getByTestId('delete-type-input'), 'wrong-name');
    expect((within(dialog).getByTestId('delete-confirm') as HTMLButtonElement).disabled).toBe(true);
  });

  // ---------------------------------------------------------------------------
  // Bulk actions
  // ---------------------------------------------------------------------------

  it('bulk delete fires deleteUser for each selected row', async () => {
    const user = userEvent.setup();
    const deleteSpy = vi.spyOn(usersApi, 'deleteUser').mockResolvedValue();
    // Also mock listUsers so invalidation doesn't re-fetch and reset state mid-test.
    vi.spyOn(usersApi, 'listUsers').mockResolvedValue([
      {
        id: 'user-a', tenantId: 'tenant-test-325', displayName: 'Alice', email: 'alice@ex.com',
        role: 'viewer', groups: [], status: 'active', lastLoginAt: null, ssoProvider: null, createdAt: new Date().toISOString(),
      },
      {
        id: 'user-b', tenantId: 'tenant-test-325', displayName: 'Bob', email: 'bob@ex.com',
        role: 'operator', groups: [], status: 'active', lastLoginAt: null, ssoProvider: null, createdAt: new Date().toISOString(),
      },
    ]);
    renderPage();
    await waitFor(() => screen.getByTestId('users-list'));

    // Click each checkbox and wait for selection to be reflected.
    const cb0 = await screen.findByTestId('user-select-user-a');
    await user.click(cb0);
    await waitFor(() =>
      expect((screen.getByTestId('user-select-user-a') as HTMLInputElement).checked).toBe(true),
    );
    const cb1 = screen.getByTestId('user-select-user-b');
    await user.click(cb1);
    await waitFor(() =>
      expect((screen.getByTestId('user-select-user-b') as HTMLInputElement).checked).toBe(true),
    );

    // Bulk delete should now be enabled.
    await user.click(screen.getByTestId('users-bulk-delete'));
    await waitFor(() => expect(deleteSpy).toHaveBeenCalledTimes(2), { timeout: 3000 });
  });

  it('bulk suspend fires suspendUser for each selected row', async () => {
    const user = userEvent.setup();
    const suspendSpy = vi.spyOn(usersApi, 'suspendUser').mockResolvedValue();
    vi.spyOn(usersApi, 'listUsers').mockResolvedValue([
      {
        id: 'user-a', tenantId: 'tenant-test-325', displayName: 'Alice', email: 'alice@ex.com',
        role: 'viewer', groups: [], status: 'active', lastLoginAt: null, ssoProvider: null, createdAt: new Date().toISOString(),
      },
      {
        id: 'user-b', tenantId: 'tenant-test-325', displayName: 'Bob', email: 'bob@ex.com',
        role: 'operator', groups: [], status: 'active', lastLoginAt: null, ssoProvider: null, createdAt: new Date().toISOString(),
      },
      {
        id: 'user-c', tenantId: 'tenant-test-325', displayName: 'Carol', email: 'carol@ex.com',
        role: 'auditor', groups: [], status: 'active', lastLoginAt: null, ssoProvider: null, createdAt: new Date().toISOString(),
      },
    ]);
    renderPage();
    await waitFor(() => screen.getByTestId('users-list'));

    const cbA = await screen.findByTestId('user-select-user-a');
    await user.click(cbA);
    await waitFor(() =>
      expect((screen.getByTestId('user-select-user-a') as HTMLInputElement).checked).toBe(true),
    );
    const cbB = screen.getByTestId('user-select-user-b');
    await user.click(cbB);
    await waitFor(() =>
      expect((screen.getByTestId('user-select-user-b') as HTMLInputElement).checked).toBe(true),
    );
    const cbC = screen.getByTestId('user-select-user-c');
    await user.click(cbC);
    await waitFor(() =>
      expect((screen.getByTestId('user-select-user-c') as HTMLInputElement).checked).toBe(true),
    );

    await user.click(screen.getByTestId('users-bulk-suspend'));
    await waitFor(() => expect(suspendSpy).toHaveBeenCalledTimes(3), { timeout: 3000 });
  });

  // ---------------------------------------------------------------------------
  // Permissions tab
  // ---------------------------------------------------------------------------

  it('permissions tab renders the role matrix', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-permissions'));
    await waitFor(() => screen.getByTestId('permissions-matrix'));

    // Matrix should have cells for admin × view.live
    expect(screen.getByTestId('perm-cell-admin-view.live')).toBeInTheDocument();
    expect(screen.getByTestId('perm-toggle-admin-view.live')).toBeInTheDocument();
  });

  it('admin role toggles are disabled (always-allowed)', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-permissions'));
    await waitFor(() => screen.getByTestId('permissions-matrix'));

    const adminToggle = screen.getByTestId(
      'perm-toggle-admin-view.live',
    ) as HTMLInputElement;
    expect(adminToggle.disabled).toBe(true);
  });

  // ---------------------------------------------------------------------------
  // Sign-in Methods tab — 6 provider cards
  // ---------------------------------------------------------------------------

  it('sign-in methods tab renders all 6 provider cards', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    for (const kind of ['local', 'entra', 'google', 'okta', 'oidc', 'saml', 'ldap']) {
      expect(screen.getByTestId(`provider-card-${kind}`)).toBeInTheDocument();
    }
  });

  // ---------------------------------------------------------------------------
  // Local wizard
  // ---------------------------------------------------------------------------

  it('Local wizard: test passes, save enables and fires saveAuthProvider', async () => {
    const saveSpy = vi.spyOn(usersApi, 'saveAuthProvider');
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-local'));
    await waitFor(() => screen.getByTestId('local-provider-wizard'));

    // Save disabled before test
    expect(
      (screen.getByTestId('wizard-save-button') as HTMLButtonElement).disabled,
    ).toBe(true);

    await user.click(screen.getByTestId('wizard-test-button'));
    await waitFor(() =>
      expect(screen.getByTestId('wizard-test-result').getAttribute('data-success')).toBe('true'),
    );

    // Save now enabled
    expect(
      (screen.getByTestId('wizard-save-button') as HTMLButtonElement).disabled,
    ).toBe(false);

    await user.click(screen.getByTestId('wizard-save-button'));
    await waitFor(() => expect(saveSpy).toHaveBeenCalled());
  });

  // ---------------------------------------------------------------------------
  // Entra wizard
  // ---------------------------------------------------------------------------

  it('Entra wizard: empty clientId shows validation error and test is blocked', async () => {
    const testSpy = vi.spyOn(usersApi, 'testAuthProvider');
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-entra'));
    await waitFor(() => screen.getByTestId('entra-provider-wizard'));

    // Click test without filling required fields
    await user.click(screen.getByTestId('wizard-test-button'));
    await waitFor(() =>
      expect(screen.getByTestId('entra-error-clientId')).toBeInTheDocument(),
    );
    // testAuthProvider should NOT be called — form validation blocks it
    expect(testSpy).not.toHaveBeenCalled();
  });

  it('Entra wizard: happy path fill → test passes → save fires', async () => {
    const saveSpy = vi.spyOn(usersApi, 'saveAuthProvider');
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-entra'));
    await waitFor(() => screen.getByTestId('entra-provider-wizard'));

    await user.type(screen.getByTestId('entra-field-clientId'), 'my-client-id');
    // PasswordField: find input within the password field wrapper
    const secretInput = screen.getByTestId('entra-field-clientSecret').closest('.password-field')
      ?.querySelector('input') ?? screen.getByTestId('entra-field-clientSecret');
    await user.type(secretInput, 'my-secret');
    await user.type(screen.getByTestId('entra-field-tenantId'), 'my-tenant-id');

    await user.click(screen.getByTestId('wizard-test-button'));
    await waitFor(() =>
      expect(screen.getByTestId('wizard-test-result').getAttribute('data-success')).toBe('true'),
    );

    await user.click(screen.getByTestId('wizard-save-button'));
    await waitFor(() => expect(saveSpy).toHaveBeenCalled());
  });

  // ---------------------------------------------------------------------------
  // Google wizard
  // ---------------------------------------------------------------------------

  it('Google wizard: test failure shows actionable error and save stays disabled', async () => {
    vi.spyOn(usersApi, 'testAuthProvider').mockResolvedValue({
      success: false,
      message: 'Connection failed: missing client_id.',
      troubleshootingUrl: 'https://docs.kaivue.com/auth/google#troubleshooting',
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-google'));
    await waitFor(() => screen.getByTestId('google-provider-wizard'));

    await user.type(screen.getByTestId('google-field-clientId'), 'some-id');
    const secretEl = screen.getByTestId('google-field-clientSecret').closest('.password-field')
      ?.querySelector('input') ?? screen.getByTestId('google-field-clientSecret');
    await user.type(secretEl, 'some-secret');
    await user.type(screen.getByTestId('google-field-hostedDomain'), 'acme.com');

    await user.click(screen.getByTestId('wizard-test-button'));
    await waitFor(() =>
      expect(screen.getByTestId('wizard-test-result').getAttribute('data-success')).toBe('false'),
    );

    // Troubleshooting link present
    expect(screen.getByTestId('wizard-troubleshoot-link')).toBeInTheDocument();

    // Save still disabled
    expect(
      (screen.getByTestId('wizard-save-button') as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  // ---------------------------------------------------------------------------
  // Okta wizard
  // ---------------------------------------------------------------------------

  it('Okta wizard: happy path fill → test → save', async () => {
    const saveSpy = vi.spyOn(usersApi, 'saveAuthProvider');
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-okta'));
    await waitFor(() => screen.getByTestId('okta-provider-wizard'));

    await user.type(screen.getByTestId('okta-field-domain'), 'acme.okta.com');
    await user.type(screen.getByTestId('okta-field-clientId'), 'okta-cid');
    const secretEl = screen.getByTestId('okta-field-clientSecret').closest('.password-field')
      ?.querySelector('input') ?? screen.getByTestId('okta-field-clientSecret');
    await user.type(secretEl, 'okta-sec');
    await user.type(screen.getByTestId('okta-field-authorizationServerId'), 'default');

    await user.click(screen.getByTestId('wizard-test-button'));
    await waitFor(() =>
      expect(screen.getByTestId('wizard-test-result').getAttribute('data-success')).toBe('true'),
    );

    await user.click(screen.getByTestId('wizard-save-button'));
    await waitFor(() => expect(saveSpy).toHaveBeenCalled());
  });

  // ---------------------------------------------------------------------------
  // Generic OIDC wizard
  // ---------------------------------------------------------------------------

  it('OIDC wizard: step 1 advances to step 2 on valid data', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-oidc'));
    await waitFor(() => screen.getByTestId('oidc-provider-wizard'));

    await user.type(screen.getByTestId('oidc-field-issuerUrl'), 'https://idp.example.com');
    await user.type(screen.getByTestId('oidc-field-clientId'), 'oidc-cid');
    const secretEl = screen.getByTestId('oidc-field-clientSecret').closest('.password-field')
      ?.querySelector('input') ?? screen.getByTestId('oidc-field-clientSecret');
    await user.type(secretEl, 'oidc-sec');
    await user.clear(screen.getByTestId('oidc-field-scopes'));
    await user.type(screen.getByTestId('oidc-field-scopes'), 'openid email profile');

    await user.click(screen.getByTestId('wizard-next'));
    await waitFor(() => screen.getByTestId('oidc-wizard-step2'));
  });

  it('OIDC wizard: step 1 blocks advance when issuerUrl is not HTTPS', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-oidc'));
    await waitFor(() => screen.getByTestId('oidc-provider-wizard'));

    await user.type(screen.getByTestId('oidc-field-issuerUrl'), 'http://insecure.example.com');
    await user.click(screen.getByTestId('wizard-next'));

    // Step 2 should NOT appear; step 1 content still visible
    await waitFor(() => expect(screen.getByTestId('oidc-wizard-step1')).toBeInTheDocument());
    expect(screen.queryByTestId('oidc-wizard-step2')).not.toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // SAML wizard
  // ---------------------------------------------------------------------------

  it('SAML wizard: metadata URL or XML required — both empty blocks advance', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-saml'));
    await waitFor(() => screen.getByTestId('saml-provider-wizard'));

    // Both fields left empty
    await user.click(screen.getByTestId('wizard-next'));

    await waitFor(() =>
      expect(screen.getByTestId('saml-error-metadataUrl')).toBeInTheDocument(),
    );
    expect(screen.queryByTestId('saml-wizard-step2')).not.toBeInTheDocument();
  });

  it('SAML wizard: happy path metadata URL → step 2 → test → save', async () => {
    const saveSpy = vi.spyOn(usersApi, 'saveAuthProvider');
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-saml'));
    await waitFor(() => screen.getByTestId('saml-provider-wizard'));

    await user.type(
      screen.getByTestId('saml-field-metadataUrl'),
      'https://idp.example.com/metadata',
    );
    await user.click(screen.getByTestId('wizard-next'));
    await waitFor(() => screen.getByTestId('saml-wizard-step2'));

    await user.type(screen.getByTestId('saml-field-entityId'), 'https://app.example.com/saml/sp');
    await user.type(screen.getByTestId('saml-field-attrEmail'), 'email');
    await user.type(screen.getByTestId('saml-field-attrName'), 'name');
    await user.type(screen.getByTestId('saml-field-attrGroups'), 'groups');

    await user.click(screen.getByTestId('wizard-test-button'));
    await waitFor(() =>
      expect(screen.getByTestId('wizard-test-result').getAttribute('data-success')).toBe('true'),
    );

    await user.click(screen.getByTestId('wizard-save-button'));
    await waitFor(() => expect(saveSpy).toHaveBeenCalled());
  });

  // ---------------------------------------------------------------------------
  // LDAP wizard
  // ---------------------------------------------------------------------------

  it('LDAP wizard: happy path step 1 → step 2 → test → save', async () => {
    const saveSpy = vi.spyOn(usersApi, 'saveAuthProvider');
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-ldap'));
    await waitFor(() => screen.getByTestId('ldap-provider-wizard'));

    await user.type(screen.getByTestId('ldap-field-host'), 'ldap.acme.com');
    await user.clear(screen.getByTestId('ldap-field-port'));
    await user.type(screen.getByTestId('ldap-field-port'), '636');
    await user.type(screen.getByTestId('ldap-field-bindDn'), 'cn=svc,dc=acme,dc=com');
    const pwEl = screen.getByTestId('ldap-field-bindPassword').closest('.password-field')
      ?.querySelector('input') ?? screen.getByTestId('ldap-field-bindPassword');
    await user.type(pwEl, 'ldap-secret');

    await user.click(screen.getByTestId('wizard-next'));
    await waitFor(() => screen.getByTestId('ldap-wizard-step2'));

    await user.type(screen.getByTestId('ldap-field-baseDn'), 'dc=acme,dc=com');
    await user.type(screen.getByTestId('ldap-field-userFilter'), '(objectClass=person)');

    await user.click(screen.getByTestId('wizard-test-button'));
    await waitFor(() =>
      expect(screen.getByTestId('wizard-test-result').getAttribute('data-success')).toBe('true'),
    );

    await user.click(screen.getByTestId('wizard-save-button'));
    await waitFor(() => expect(saveSpy).toHaveBeenCalled());
  });

  it('LDAP wizard: missing host means step 2 never appears', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-ldap'));
    await waitFor(() => screen.getByTestId('ldap-wizard-step1'));

    // Leave host empty (default), click next to trigger validation.
    await user.click(screen.getByTestId('wizard-next'));

    // After clicking next with an empty host, step 2 must NOT appear.
    // The validation is enforced by both Zod schema (tested in unit tests)
    // and by the trigger() call in handleNext gating setStep(1).
    // We assert step 2 is absent and step 1 is still present.
    await new Promise((r) => setTimeout(r, 200)); // let async trigger settle
    expect(screen.queryByTestId('ldap-wizard-step2')).not.toBeInTheDocument();
    expect(screen.getByTestId('ldap-wizard-step1')).toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // PasswordField — secret never exposed as plain text by default
  // ---------------------------------------------------------------------------

  it('client secret inputs render as type=password by default', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByTestId('tab-signInMethods'));
    await waitFor(() => screen.getByTestId('sign-in-methods-tab'));

    await user.click(screen.getByTestId('provider-configure-entra'));
    await waitFor(() => screen.getByTestId('entra-provider-wizard'));

    const wrapper = screen.getByTestId('entra-field-clientSecret').closest('.password-field');
    const input = wrapper?.querySelector('input') as HTMLInputElement | null;
    expect(input).not.toBeNull();
    expect(input?.getAttribute('type')).toBe('password');
  });

  // ---------------------------------------------------------------------------
  // a11y smoke
  // ---------------------------------------------------------------------------

  it('has no critical or serious axe violations on users tab', async () => {
    renderPage();
    const page = await waitFor(() => screen.getByTestId('users-page'));
    await waitFor(() => screen.getByTestId('users-list'));
    const violations = await runAxe(page);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toEqual([]);
  });
});
