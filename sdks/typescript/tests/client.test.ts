import { KaiVueClient, CameraState, NotFoundError, AuthenticationError } from "../src";

// Mock fetch globally
const mockFetch = jest.fn();
global.fetch = mockFetch as unknown as typeof fetch;

const BASE_URL = "https://test.kaivue.io";

function mockResponse(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Map(Object.entries(headers)),
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

beforeEach(() => {
  mockFetch.mockReset();
});

describe("KaiVueClient", () => {
  it("throws if both apiKey and auth are provided", () => {
    expect(() => {
      new KaiVueClient(BASE_URL, {
        apiKey: "k",
        auth: { apply: () => {} },
      });
    }).toThrow("not both");
  });
});

describe("CameraService", () => {
  it("lists cameras", async () => {
    mockFetch.mockResolvedValueOnce(
      mockResponse({
        cameras: [
          {
            id: "cam-001",
            name: "Front Door",
            state: "CAMERA_STATE_ONLINE",
            recording_mode: "RECORDING_MODE_CONTINUOUS",
          },
        ],
        next_cursor: "",
        total_count: 1,
      })
    );

    const client = new KaiVueClient(BASE_URL, { apiKey: "test-key" });
    const resp = await client.cameras.list();

    expect(resp.cameras).toHaveLength(1);
    expect(resp.cameras[0].name).toBe("Front Door");
    expect(resp.cameras[0].state).toBe(CameraState.Online);

    // Verify API key was sent
    const [url, opts] = mockFetch.mock.calls[0];
    expect(opts.headers["X-API-Key"]).toBe("test-key");
  });

  it("gets a camera by id", async () => {
    mockFetch.mockResolvedValueOnce(
      mockResponse({
        camera: { id: "cam-001", name: "Front Door" },
      })
    );

    const client = new KaiVueClient(BASE_URL, { apiKey: "k" });
    const cam = await client.cameras.get("cam-001");

    expect(cam.id).toBe("cam-001");
    expect(cam.name).toBe("Front Door");
  });

  it("creates a camera", async () => {
    mockFetch.mockResolvedValueOnce(
      mockResponse({
        camera: { id: "cam-new", name: "New Camera" },
      })
    );

    const client = new KaiVueClient(BASE_URL, { apiKey: "k" });
    const cam = await client.cameras.create({
      name: "New Camera",
      ip_address: "192.168.1.10",
      recorder_id: "rec-01",
    });

    expect(cam.name).toBe("New Camera");
  });
});

describe("UserService", () => {
  it("lists users", async () => {
    mockFetch.mockResolvedValueOnce(
      mockResponse({
        users: [{ id: "usr-001", username: "jdoe", email: "jdoe@example.com" }],
        next_cursor: "",
        total_count: 1,
      })
    );

    const client = new KaiVueClient(BASE_URL, { apiKey: "k" });
    const resp = await client.users.list();

    expect(resp.users).toHaveLength(1);
    expect(resp.users[0].username).toBe("jdoe");
  });
});

describe("Error handling", () => {
  it("throws NotFoundError on 404", async () => {
    mockFetch.mockResolvedValueOnce(
      mockResponse(
        { message: "Camera not found", request_id: "req-123" },
        404,
        { "X-Request-Id": "req-123" }
      )
    );

    const client = new KaiVueClient(BASE_URL, { apiKey: "k" });
    await expect(client.cameras.get("missing")).rejects.toThrow(NotFoundError);
  });

  it("throws AuthenticationError on 401", async () => {
    mockFetch.mockResolvedValueOnce(
      mockResponse({ message: "Invalid API key" }, 401)
    );

    const client = new KaiVueClient(BASE_URL, { apiKey: "bad" });
    await expect(client.cameras.list()).rejects.toThrow(AuthenticationError);
  });
});
