export class ApiClient {
  constructor(baseUrl = "") {
    this.baseUrl = baseUrl;
    this.token = "";
  }

  setToken(token) {
    this.token = token || "";
  }

  async login(password) {
    return this.#request("POST", "/api/v1/operator/login", { password }, false);
  }

  async capabilities() {
    return this.#request("GET", "/api/v1/operator/capabilities");
  }

  async listDevices() {
    return this.#request("GET", "/api/v1/devices");
  }

  async getDevice(deviceId) {
    return this.#request("GET", `/api/v1/devices/${encodeURIComponent(deviceId)}`);
  }

  async listTelemetry(deviceId, limit = 100) {
    return this.#request(
      "GET",
      `/api/v1/devices/${encodeURIComponent(deviceId)}/telemetry?limit=${limit}`
    );
  }

  async listCommands(deviceId, limit = 100) {
    return this.#request(
      "GET",
      `/api/v1/devices/${encodeURIComponent(deviceId)}/commands?limit=${limit}`
    );
  }

  async createCommand(payload) {
    return this.#request("POST", "/api/v1/commands", payload);
  }

  async uploadArtifact(payload) {
    return this.#request("POST", "/api/v1/artifacts", payload);
  }

  async #request(method, path, body, withAuth = true) {
    const headers = {
      "Content-Type": "application/json",
    };

    if (withAuth && this.token) {
      headers.Authorization = `Bearer ${this.token}`;
    }

    const response = await fetch(this.baseUrl + path, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });

    const isJSON = (response.headers.get("content-type") || "").includes("application/json");
    const data = isJSON ? await response.json() : null;

    if (!response.ok) {
      const message = data?.error || `${response.status} ${response.statusText}`;
      throw new Error(message);
    }

    return data;
  }
}

export async function fileToBase64(file) {
  const buffer = await file.arrayBuffer();
  const bytes = new Uint8Array(buffer);

  let binary = "";
  for (let i = 0; i < bytes.byteLength; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }

  return btoa(binary);
}
