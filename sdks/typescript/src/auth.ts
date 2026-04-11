/** Authentication providers for the KaiVue SDK. */

export interface AuthProvider {
  /** Apply authentication to request headers. */
  apply(headers: Record<string, string>): void;
}

/** Authenticate via a static API key. */
export class APIKeyAuth implements AuthProvider {
  constructor(private apiKey: string) {}

  apply(headers: Record<string, string>): void {
    headers["X-API-Key"] = this.apiKey;
  }
}

/** Authenticate via an OAuth2 bearer token. */
export class OAuthAuth implements AuthProvider {
  private accessToken: string;
  private refreshToken?: string;
  private tokenUrl?: string;
  private clientId?: string;
  private clientSecret?: string;
  private expiresAt = 0;

  constructor(opts: {
    accessToken?: string;
    refreshToken?: string;
    tokenUrl?: string;
    clientId?: string;
    clientSecret?: string;
  }) {
    this.accessToken = opts.accessToken ?? "";
    this.refreshToken = opts.refreshToken;
    this.tokenUrl = opts.tokenUrl;
    this.clientId = opts.clientId;
    this.clientSecret = opts.clientSecret;
  }

  apply(headers: Record<string, string>): void {
    headers["Authorization"] = `Bearer ${this.accessToken}`;
  }

  /** Refresh the access token if needed. Call before apply() if using auto-refresh. */
  async refreshIfNeeded(): Promise<void> {
    if (this.accessToken && Date.now() / 1000 < this.expiresAt - 30) {
      return;
    }
    if (!this.tokenUrl) {
      throw new Error("Cannot refresh: no tokenUrl configured");
    }

    const body = new URLSearchParams();
    if (this.refreshToken) {
      body.set("grant_type", "refresh_token");
      body.set("refresh_token", this.refreshToken);
    } else if (this.clientId && this.clientSecret) {
      body.set("grant_type", "client_credentials");
      body.set("client_id", this.clientId);
      body.set("client_secret", this.clientSecret);
    } else {
      throw new Error("Cannot refresh: need refreshToken or clientId+clientSecret");
    }

    const resp = await fetch(this.tokenUrl, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: body.toString(),
    });
    if (!resp.ok) {
      throw new Error(`Token refresh failed: ${resp.status}`);
    }
    const data = await resp.json();
    this.accessToken = data.access_token;
    if (data.refresh_token) {
      this.refreshToken = data.refresh_token;
    }
    if (data.expires_in) {
      this.expiresAt = Date.now() / 1000 + data.expires_in;
    }
  }
}
