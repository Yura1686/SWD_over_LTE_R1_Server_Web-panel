const ENCODER = new TextEncoder();
const DECODER = new TextDecoder();

const USB_READ_BYTES = 512;
const IO_BAUDRATE = 115200;
const EVENT_TIMEOUT_MS = 7000;
const SERIAL_POLL_MS = 25;
const COMMAND_RETRIES = 3;
const RETRY_DELAY_MS = 220;

function sanitizeValue(value) {
  return String(value ?? "").replace(/[;\r\n]/g, "").trim();
}

function hex16(value) {
  return Number(value || 0)
    .toString(16)
    .padStart(4, "0");
}

function sleep(ms) {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

function isProtectedUsbInterfaceError(error) {
  const text = String(error?.message || error || "").toLowerCase();
  return text.includes("protected") || text.includes("claim") || text.includes("access denied") || text.includes("busy");
}

// ProvisioningUSB uses Web Serial for RP2040 CDC transport and keeps WebUSB as fallback.
export class ProvisioningUSB {
  constructor() {
    this.mode = "";
    this.currentDeviceName = "";

    this.device = null;
    this.iface = 0;
    this.outEndpoint = 0;
    this.inEndpoint = 0;

    this.port = null;
    this.serialReader = null;
    this.serialWriter = null;
    this.serialReadTask = null;

    this.readBuffer = "";
    this.eventQueue = [];
    this.deviceByKey = new Map();
    this.portByKey = new Map();
  }

  get webusbSupported() {
    return typeof navigator !== "undefined" && "usb" in navigator;
  }

  get webserialSupported() {
    return typeof navigator !== "undefined" && "serial" in navigator;
  }

  get supported() {
    return this.webusbSupported || this.webserialSupported;
  }

  get connected() {
    if (this.mode === "webserial") {
      return Boolean(this.port && this.port.readable && this.port.writable && this.serialReader && this.serialWriter);
    }
    if (this.mode === "webusb") {
      return Boolean(this.device && this.device.opened);
    }
    return false;
  }

  isCurrentDevice(device) {
    return this.mode === "webusb" && Boolean(this.device && device && this.device === device);
  }

  async listKnownDevices() {
    this.deviceByKey.clear();
    this.portByKey.clear();

    const results = [];

    if (this.webserialSupported) {
      const ports = await navigator.serial.getPorts();
      const seen = new Map();

      ports.forEach((port) => {
        const info = typeof port.getInfo === "function" ? port.getInfo() : {};
        const vid = hex16(info.usbVendorId);
        const pid = hex16(info.usbProductId);
        const baseKey = `serial-vid-${vid}-pid-${pid}`;
        const repeat = seen.get(baseKey) || 0;
        const key = repeat === 0 ? baseKey : `${baseKey}#${repeat}`;
        seen.set(baseKey, repeat + 1);

        this.portByKey.set(key, port);
        results.push({
          key,
          productName: "Serial USB Device",
          label: `Serial USB Device [${vid}:${pid}]`,
          connected: this.mode === "webserial" && this.port === port,
        });
      });

      // RP2040 provisioning uses USB CDC, so serial entries are authoritative.
      return results;
    }

    if (this.webusbSupported) {
      const devices = await navigator.usb.getDevices();
      const seen = new Map();

      devices.forEach((device) => {
        const keyBase = this.#baseUsbKey(device);
        const repeat = seen.get(keyBase) || 0;
        const key = repeat === 0 ? keyBase : `${keyBase}#${repeat}`;
        seen.set(keyBase, repeat + 1);

        const productName = device.productName || "USB Device";
        const vid = hex16(device.vendorId);
        const pid = hex16(device.productId);
        const serial = device.serialNumber ? ` SN:${device.serialNumber}` : "";

        this.deviceByKey.set(key, device);
        results.push({
          key,
          productName,
          label: `${productName} [${vid}:${pid}]${serial}`,
          connected: this.mode === "webusb" && this.device === device,
        });
      });
    }

    return results;
  }

  async pairAndConnect() {
    if (!this.supported) {
      throw new Error("Browser does not support USB transport APIs");
    }

    // RP2040 firmware exposes provisioning over USB CDC, so Web Serial is preferred.
    if (this.webserialSupported) {
      const port = await navigator.serial.requestPort({});
      return this.#openSerialPort(port);
    }

    const device = await navigator.usb.requestDevice({
      filters: [],
      acceptAllDevices: true,
    });
    return this.#openUsbDevice(device);
  }

  async connectByKey(key) {
    if (!this.supported) {
      throw new Error("Browser does not support USB transport APIs");
    }

    if (!key) {
      throw new Error("USB device is not selected");
    }

    if (key.startsWith("serial-")) {
      let port = this.portByKey.get(key);
      if (!port) {
        await this.listKnownDevices();
        port = this.portByKey.get(key);
      }
      if (!port) {
        throw new Error("Selected serial USB device is not available");
      }
      return this.#openSerialPort(port);
    }

    if (key.startsWith("usb-")) {
      let device = this.deviceByKey.get(key);
      if (!device) {
        await this.listKnownDevices();
        device = this.deviceByKey.get(key);
      }
      if (!device) {
        throw new Error("Selected USB device is not available");
      }
      return this.#openUsbDevice(device);
    }

    throw new Error("Unknown USB device type");
  }

  async disconnect() {
    await this.#closeSerial();
    await this.#closeWebUsb();

    this.mode = "";
    this.currentDeviceName = "";
    this.iface = 0;
    this.outEndpoint = 0;
    this.inEndpoint = 0;
    this.readBuffer = "";
    this.eventQueue = [];
  }

  async setConfig(config) {
    this.#ensureConnected();

    const fields = [
      `device_id=${sanitizeValue(config.device_id)}`,
      `apn=${sanitizeValue(config.apn)}`,
      `operator=${sanitizeValue(config.operator)}`,
      `sim_pin=${sanitizeValue(config.sim_pin)}`,
      `server_url=${sanitizeValue(config.server_url)}`,
      `enroll_key=${sanitizeValue(config.enroll_key)}`,
      `password=${sanitizeValue(config.password || "r1-config")}`,
    ];

    const response = await this.#executeCommandWithRetry(`SET ${fields.join(";")}`, "set_config");
    if (!response.success) {
      throw new Error(response.error || "set_config failed");
    }
    return response;
  }

  async getConfig(password) {
    this.#ensureConnected();

    const response = await this.#executeCommandWithRetry(`GET password=${sanitizeValue(password)}`, "get_config");
    if (!response.success) {
      throw new Error(response.error || "get_config failed");
    }
    return response.config || {};
  }

  async #executeCommandWithRetry(commandLine, eventName) {
    let lastError = null;

    for (let attempt = 1; attempt <= COMMAND_RETRIES; attempt += 1) {
      this.#clearPendingEvents();
      this.readBuffer = "";

      try {
        await this.#sendLine(commandLine);
        return await this.#waitForEvent(eventName, EVENT_TIMEOUT_MS);
      } catch (error) {
        lastError = error;
        if (!this.connected) {
          break;
        }
        if (attempt < COMMAND_RETRIES) {
          await sleep(RETRY_DELAY_MS);
        }
      }
    }

    throw lastError || new Error(`failed to execute ${eventName}`);
  }

  async #openSerialPort(port) {
    if (!port) {
      throw new Error("Serial USB device is not selected");
    }

    // The module reuses existing lock if the same port is already connected.
    if (this.mode === "webserial" && this.port === port && this.connected) {
      const knownConnected = await this.listKnownDevices();
      const selectedConnected = knownConnected.find((item) => this.portByKey.get(item.key) === this.port);
      this.currentDeviceName = selectedConnected?.productName || "Serial USB Device";
      return {
        mode: "webserial",
        key: selectedConnected?.key || this.#baseSerialKey(this.port),
        productName: this.currentDeviceName,
      };
    }

    if ((this.mode === "webserial" && this.port !== port) || this.mode === "webusb") {
      await this.disconnect();
    }

    this.mode = "webserial";
    this.port = port;
    this.readBuffer = "";
    this.eventQueue = [];

    if (!this.port.readable || !this.port.writable) {
      await this.port.open({
        baudRate: IO_BAUDRATE,
        dataBits: 8,
        stopBits: 1,
        parity: "none",
        flowControl: "none",
      });
    }

    try {
      await this.port.setSignals({
        dataTerminalReady: true,
        requestToSend: true,
      });
    } catch {
      // The module ignores signal errors because some platforms/drivers do not expose these controls.
    }

    try {
      this.serialReader = this.port.readable.getReader();
      this.serialWriter = this.port.writable.getWriter();
    } catch {
      throw new Error("Cannot open serial stream. The port may be used by another tab or application.");
    }

    this.#startSerialReadPump();

    const known = await this.listKnownDevices();
    const selected = known.find((item) => this.portByKey.get(item.key) === this.port);
    this.currentDeviceName = selected?.productName || "Serial USB Device";

    return {
      mode: "webserial",
      key: selected?.key || this.#baseSerialKey(this.port),
      productName: this.currentDeviceName,
    };
  }

  async #openUsbDevice(device) {
    if (!device) {
      throw new Error("USB device is not selected");
    }

    if ((this.mode === "webusb" && this.device !== device) || this.mode === "webserial") {
      await this.disconnect();
    }

    this.mode = "webusb";
    this.device = device;
    this.readBuffer = "";
    this.eventQueue = [];

    if (!this.device.opened) {
      await this.device.open();
    }

    if (this.device.configuration === null) {
      await this.device.selectConfiguration(1);
    }

    const channel = this.#pickUsbInterface(this.device.configuration);
    if (!channel) {
      throw new Error("No compatible USB interface with IN/OUT endpoints");
    }

    this.iface = channel.interfaceNumber;
    this.outEndpoint = channel.outEndpoint;
    this.inEndpoint = channel.inEndpoint;

    try {
      await this.device.claimInterface(this.iface);
      if (typeof channel.alternateSetting === "number") {
        try {
          await this.device.selectAlternateInterface(this.iface, channel.alternateSetting);
        } catch {
          // The module ignores alternate-switch errors because many devices expose only alt=0.
        }
      }
    } catch (error) {
      await this.#closeWebUsb();
      this.mode = "";
      if (isProtectedUsbInterfaceError(error)) {
        throw new Error("The device uses a protected USB class. Select Serial USB Device in the list.");
      }
      throw error;
    }

    this.currentDeviceName = this.device.productName || "USB Device";
    const known = await this.listKnownDevices();
    const selected = known.find((item) => this.deviceByKey.get(item.key) === this.device);

    return {
      mode: "webusb",
      key: selected?.key || this.#baseUsbKey(this.device),
      productName: this.currentDeviceName,
    };
  }

  #pickUsbInterface(configuration) {
    if (!configuration?.interfaces) {
      return null;
    }

    for (const iface of configuration.interfaces) {
      const alternates =
        Array.isArray(iface.alternates) && iface.alternates.length > 0
          ? iface.alternates
          : iface.alternate
            ? [iface.alternate]
            : [];

      for (const alt of alternates) {
        const endpoints = alt.endpoints || [];
        const out = endpoints.find((ep) => ep.direction === "out");
        const input = endpoints.find((ep) => ep.direction === "in");
        if (out && input) {
          return {
            interfaceNumber: iface.interfaceNumber,
            alternateSetting: alt.alternateSetting || 0,
            outEndpoint: out.endpointNumber,
            inEndpoint: input.endpointNumber,
          };
        }
      }
    }

    return null;
  }

  #ensureConnected() {
    if (!this.connected) {
      throw new Error("USB device is not connected");
    }
  }

  async #sendLine(line) {
    const payload = ENCODER.encode(`${line}\n`);

    if (this.mode === "webserial") {
      if (!this.serialWriter) {
        throw new Error("Serial writer is unavailable");
      }
      await this.serialWriter.write(payload);
      return;
    }

    if (this.mode === "webusb") {
      if (!this.device || !this.outEndpoint) {
        throw new Error("WebUSB endpoint is unavailable");
      }
      await this.device.transferOut(this.outEndpoint, payload);
      return;
    }

    throw new Error("USB transport mode is not selected");
  }

  #startSerialReadPump() {
    if (this.serialReadTask || !this.serialReader) {
      return;
    }

    this.serialReadTask = (async () => {
      while (this.mode === "webserial" && this.serialReader) {
        let result = null;
        try {
          result = await this.serialReader.read();
        } catch {
          break;
        }

        if (!result) {
          continue;
        }
        if (result.done) {
          break;
        }
        if (!result.value || result.value.length === 0) {
          continue;
        }

        const text = DECODER.decode(result.value, { stream: true });
        this.#ingestChunk(text);
      }
    })().finally(() => {
      this.serialReadTask = null;
    });
  }

  async #waitForEvent(eventName, timeoutMs) {
    if (this.mode === "webserial") {
      return this.#waitForEventSerial(eventName, timeoutMs);
    }
    if (this.mode === "webusb") {
      return this.#waitForEventWebUsb(eventName, timeoutMs);
    }
    throw new Error("USB transport mode is not selected");
  }

  async #waitForEventSerial(eventName, timeoutMs) {
    const deadline = Date.now() + timeoutMs;

    while (Date.now() < deadline) {
      const queued = this.#dequeueEvent(eventName);
      if (queued) {
        return queued;
      }

      const fatal = this.#dequeueFatalEvent();
      if (fatal) {
        throw new Error(fatal.message || fatal.error || `${fatal.usb_event}`);
      }

      if (!this.connected) {
        throw new Error("Serial USB device disconnected");
      }

      await sleep(SERIAL_POLL_MS);
    }

    throw new Error(`timeout waiting for ${eventName}`);
  }

  async #waitForEventWebUsb(eventName, timeoutMs) {
    const deadline = Date.now() + timeoutMs;

    while (Date.now() < deadline) {
      const queuedBeforeRead = this.#dequeueEvent(eventName);
      if (queuedBeforeRead) {
        return queuedBeforeRead;
      }

      const fatalBeforeRead = this.#dequeueFatalEvent();
      if (fatalBeforeRead) {
        throw new Error(fatalBeforeRead.message || fatalBeforeRead.error || `${fatalBeforeRead.usb_event}`);
      }

      let result = null;
      try {
        result = await this.device.transferIn(this.inEndpoint, USB_READ_BYTES);
      } catch {
        if (!this.connected) {
          throw new Error("USB device disconnected");
        }
        continue;
      }

      if (!result?.data) {
        continue;
      }

      const bytes = new Uint8Array(result.data.buffer, result.data.byteOffset, result.data.byteLength);
      const text = DECODER.decode(bytes);
      this.#ingestChunk(text);

      const queuedAfterRead = this.#dequeueEvent(eventName);
      if (queuedAfterRead) {
        return queuedAfterRead;
      }

      const fatalAfterRead = this.#dequeueFatalEvent();
      if (fatalAfterRead) {
        throw new Error(fatalAfterRead.message || fatalAfterRead.error || `${fatalAfterRead.usb_event}`);
      }
    }

    throw new Error(`timeout waiting for ${eventName}`);
  }

  #ingestChunk(chunk) {
    this.readBuffer += chunk;

    // The buffer is bounded to avoid unbounded growth when logs are noisy.
    if (this.readBuffer.length > 8192) {
      this.readBuffer = this.readBuffer.slice(-4096);
    }

    while (true) {
      const newlineIndex = this.readBuffer.indexOf("\n");
      if (newlineIndex < 0) {
        return;
      }

      const rawLine = this.readBuffer.slice(0, newlineIndex);
      this.readBuffer = this.readBuffer.slice(newlineIndex + 1);
      const line = rawLine.replace(/\r/g, "").trim();
      if (!line) {
        continue;
      }

      let payload = null;
      try {
        payload = JSON.parse(line);
      } catch {
        payload = null;
      }

      if (!payload && line.includes('"usb_event":"get_config"')) {
        this.eventQueue.push({
          usb_event: "get_config",
          success: false,
          error: "malformed_get_config_response",
          message: "Device returned truncated get_config response. Flash latest firmware UF2.",
        });
        continue;
      }

      if (!payload && line.includes('"usb_event":"set_config"')) {
        this.eventQueue.push({
          usb_event: "set_config",
          success: false,
          error: "malformed_set_config_response",
          message: "Device returned truncated set_config response. Flash latest firmware UF2.",
        });
        continue;
      }

      if (!payload || typeof payload !== "object") {
        continue;
      }

      if (typeof payload.usb_event === "string") {
        this.eventQueue.push(payload);
        if (this.eventQueue.length > 64) {
          this.eventQueue.shift();
        }
      }
    }
  }

  #dequeueEvent(eventName) {
    const eventIndex = this.eventQueue.findIndex((entry) => entry.usb_event === eventName);
    if (eventIndex < 0) {
      return null;
    }

    const [event] = this.eventQueue.splice(eventIndex, 1);
    return event || null;
  }

  #dequeueFatalEvent() {
    const fatalEvents = new Set(["error", "unknown_command"]);
    const eventIndex = this.eventQueue.findIndex((entry) => fatalEvents.has(entry.usb_event));
    if (eventIndex < 0) {
      return null;
    }

    const [event] = this.eventQueue.splice(eventIndex, 1);
    return event || null;
  }

  #clearPendingEvents() {
    if (!Array.isArray(this.eventQueue) || this.eventQueue.length === 0) {
      return;
    }
    this.eventQueue = [];
  }

  async #closeSerial() {
    if (!this.port) {
      this.serialReader = null;
      this.serialWriter = null;
      this.serialReadTask = null;
      return;
    }

    if (this.serialReader) {
      try {
        await this.serialReader.cancel();
      } catch {
        // The module ignores cancel errors because reader can already be closed.
      }
      try {
        this.serialReader.releaseLock();
      } catch {
        // The module ignores lock-release errors because lock can already be released.
      }
      this.serialReader = null;
    }

    if (this.serialWriter) {
      try {
        this.serialWriter.releaseLock();
      } catch {
        // The module ignores lock-release errors because lock can already be released.
      }
      this.serialWriter = null;
    }

    if (this.serialReadTask) {
      try {
        await this.serialReadTask;
      } catch {
        // The module ignores reader-loop errors because disconnect is explicit.
      }
      this.serialReadTask = null;
    }

    try {
      await this.port.close();
    } catch {
      // The module ignores close errors because browser can close port implicitly.
    }

    this.port = null;
  }

  async #closeWebUsb() {
    if (!this.device) {
      return;
    }

    try {
      if (this.device.opened) {
        await this.device.close();
      }
    } catch {
      // The module ignores close errors because browser state can already be closed.
    }

    this.device = null;
  }

  #baseUsbKey(device) {
    const serial = device.serialNumber ? `sn-${device.serialNumber}` : "sn-none";
    return `usb-vid-${hex16(device.vendorId)}-pid-${hex16(device.productId)}-${serial}`;
  }

  #baseSerialKey(port) {
    const info = typeof port.getInfo === "function" ? port.getInfo() : {};
    return `serial-vid-${hex16(info.usbVendorId)}-pid-${hex16(info.usbProductId)}`;
  }
}
