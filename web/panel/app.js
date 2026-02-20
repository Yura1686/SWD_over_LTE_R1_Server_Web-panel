import { ApiClient, fileToBase64 } from "./api.js";
import { DeviceMap } from "./map.js";
import { ProvisioningUSB } from "./webusb.js";

const state = {
  token: "",
  devices: [],
  selectedDeviceId: "",
  selectedDevice: null,
  commands: [],
  supportedCommands: [],
  refreshTimer: null,
  refreshInFlight: false,
};

const api = new ApiClient("");
const mapView = new DeviceMap("map");
const usb = new ProvisioningUSB();

const loginSection = document.getElementById("loginSection");
const appSection = document.getElementById("appSection");

const loginForm = document.getElementById("loginForm");
const loginError = document.getElementById("loginError");
const connectionState = document.getElementById("connectionState");

const ledCloud = document.getElementById("ledCloud");
const ledModem = document.getElementById("ledModem");
const ledCommand = document.getElementById("ledCommand");

const deviceList = document.getElementById("deviceList");
const deviceDetail = document.getElementById("deviceDetail");
const commandHistory = document.getElementById("commandHistory");
const commandType = document.getElementById("commandType");
const commandForm = document.getElementById("commandForm");
const commandResult = document.getElementById("commandResult");

const artifactForm = document.getElementById("artifactForm");
const artifactResult = document.getElementById("artifactResult");

const refreshBtn = document.getElementById("refreshBtn");

const ledUsb = document.getElementById("ledUsb");
const usbSupportNotice = document.getElementById("usbSupportNotice");
const usbDeviceSelect = document.getElementById("usbDeviceSelect");
const usbRefreshBtn = document.getElementById("usbRefreshBtn");
const usbPairBtn = document.getElementById("usbPairBtn");
const usbConnectBtn = document.getElementById("usbConnectBtn");
const usbDisconnectBtn = document.getElementById("usbDisconnectBtn");
const usbState = document.getElementById("usbState");
const usbSetForm = document.getElementById("usbSetForm");
const usbGetForm = document.getElementById("usbGetForm");
const usbConfigView = document.getElementById("usbConfigView");
const usbServerUrlInput = document.getElementById("usbServerUrl");
const usbEnrollKeyInput = document.getElementById("usbEnrollKey");

const USB_PREF_SERVER_URL = "lte_swd_usb_server_url";
const USB_PREF_ENROLL_KEY = "lte_swd_usb_enroll_key";

setLedState(ledCloud, "off", false);
setLedState(ledModem, "off", false);
setLedState(ledCommand, "off", false);
setLedState(ledUsb, "off", false);
initializeUsbPanel();

loginForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  loginError.textContent = "";

  const formData = new FormData(loginForm);
  const password = String(formData.get("password") || "").trim();

  try {
    const response = await api.login(password);
    state.token = response.token;
    api.setToken(state.token);

    loginSection.classList.add("hidden");
    appSection.classList.remove("hidden");
    mapView.resize();

    connectionState.textContent = "online";
    setLedState(ledCloud, "green", false);

    await loadCapabilities();
    await refreshAll();
    scheduleRefresh();
  } catch (error) {
    loginError.textContent = String(error.message || error);
    setLedState(ledCloud, "red", true);
  }
});

refreshBtn.addEventListener("click", () => {
  refreshAll();
});

commandForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  commandResult.textContent = "";

  if (!state.selectedDeviceId) {
    commandResult.textContent = "Select a device first";
    setLedState(ledCommand, "yellow", true);
    return;
  }

  try {
    const payloadRaw = document.getElementById("commandPayload").value || "{}";
    const payload = JSON.parse(payloadRaw);

    setLedState(ledCommand, "yellow", true);
    const command = await api.createCommand({
      device_id: state.selectedDeviceId,
      type: commandType.value,
      payload,
    });

    commandResult.textContent = `Command ${command.command_id} queued`;
    await refreshCommands();
  } catch (error) {
    commandResult.textContent = String(error.message || error);
    setLedState(ledCommand, "red", true);
  }
});

artifactForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  artifactResult.textContent = "";

  try {
    const fileInput = document.getElementById("artifactFile");
    const nameInput = document.getElementById("artifactName");
    const file = fileInput.files?.[0];
    if (!file) {
      throw new Error("No file selected");
    }

    const base64Data = await fileToBase64(file);
    const response = await api.uploadArtifact({
      name: nameInput.value || file.name,
      content_type: file.type || "application/octet-stream",
      base64_data: base64Data,
    });

    artifactResult.textContent = `artifact_id: ${response.artifact_id} (sha256: ${response.payload_sha256})`;
  } catch (error) {
    artifactResult.textContent = String(error.message || error);
  }
});

usbRefreshBtn.addEventListener("click", async () => {
  await refreshUsbDeviceList();
});

usbDeviceSelect.addEventListener("change", () => {
  updateUsbConnectButtonState();
});

usbPairBtn.addEventListener("click", async () => {
  if (!usb.supported) {
    return;
  }

  try {
    setUsbStatus("yellow", "waiting for device selection...");
    const info = await usb.pairAndConnect();
    await refreshUsbDeviceList(info.key);
    setUsbConnected(info.productName, info.mode);
  } catch (error) {
    if (error?.name === "NotFoundError") {
      setUsbDisconnected();
      return;
    }
    setUsbStatus("red", "error");
    usbConfigView.textContent = String(error.message || error);
  }
});

usbConnectBtn.addEventListener("click", async () => {
  if (!usb.supported) {
    return;
  }

  const key = String(usbDeviceSelect.value || "");
  if (!key) {
    usbConfigView.textContent = "Select a device from the list first";
    return;
  }

  try {
    setUsbStatus("yellow", "connecting...");
    const info = await usb.connectByKey(key);
    await refreshUsbDeviceList(info.key);
    setUsbConnected(info.productName, info.mode);
  } catch (error) {
    setUsbStatus("red", "error");
    usbConfigView.textContent = String(error.message || error);
  }
});

usbDisconnectBtn.addEventListener("click", async () => {
  try {
    await usb.disconnect();
    await refreshUsbDeviceList();
    setUsbDisconnected();
  } catch (error) {
    setUsbStatus("red", "error");
    usbConfigView.textContent = String(error.message || error);
  }
});

usbSetForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const formData = new FormData(usbSetForm);

  try {
    setUsbStatus("yellow", "writing...");
    const passwordField = document.getElementById("usbConfigPassword");
    const effectivePassword = String(passwordField?.value || "r1-config");
    if (passwordField && !passwordField.value) {
      passwordField.value = effectivePassword;
    }

    const config = {
      device_id: String(formData.get("usbDeviceId") || ""),
      apn: String(formData.get("usbApn") || ""),
      operator: String(formData.get("usbOperator") || ""),
      sim_pin: String(formData.get("usbSimPin") || ""),
      server_url: String(formData.get("usbServerUrl") || ""),
      enroll_key: String(formData.get("usbEnrollKey") || ""),
      password: effectivePassword,
    };

    if (!config.device_id || !config.apn || !config.operator || !config.server_url || !config.enroll_key) {
      throw new Error("Device ID, APN, Operator, Server URL, and Enroll Key are required.");
    }
    if (
      config.device_id.length > 31 ||
      config.apn.length > 47 ||
      config.operator.length > 31 ||
      config.sim_pin.length > 15 ||
      config.server_url.length > 95 ||
      config.enroll_key.length > 47 ||
      config.password.length > 31
    ) {
      throw new Error("One or more fields exceed the supported firmware limits.");
    }

    const response = await usb.setConfig(config);
    const verifiedConfig = await usb.getConfig(config.password);
    window.localStorage.setItem(USB_PREF_SERVER_URL, config.server_url);
    window.localStorage.setItem(USB_PREF_ENROLL_KEY, config.enroll_key);
    usbConfigView.textContent = JSON.stringify(
      {
        set_config: response,
        verify_get_config: verifiedConfig,
      },
      null,
      2,
    );
    setUsbConnected(usb.currentDeviceName || "USB Device", usb.mode);
  } catch (error) {
    setUsbStatus("red", "error");
    usbConfigView.textContent = String(error.message || error);
  }
});

usbGetForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const formData = new FormData(usbGetForm);

  try {
    setUsbStatus("yellow", "reading...");
    const password = String(formData.get("usbConfigPassword") || "");
    const config = await usb.getConfig(password);
    usbConfigView.textContent = JSON.stringify(config, null, 2);
    setUsbConnected(usb.currentDeviceName || "USB Device", usb.mode);
  } catch (error) {
    setUsbStatus("red", "error");
    usbConfigView.textContent = String(error.message || error);
  }
});

async function initializeUsbPanel() {
  setUsbDiscoveryEnabled(false);
  setUsbProvisioningEnabled(false);
  setUsbDisconnected();
  preloadUsbDefaults();

  if (!usb.supported) {
    if (!window.isSecureContext) {
      usbSupportNotice.textContent =
        "WebUSB is blocked: page is not in a secure context. Use HTTPS for 192.168.x.x (or localhost).";
    } else {
      usbSupportNotice.textContent =
        "WebUSB/Web Serial is unavailable in this browser. Use Chromium, Chrome, or Edge.";
    }
    usbSupportNotice.classList.remove("hidden");
    setUsbStatus("red", "unsupported");
    return;
  }

  usbSupportNotice.classList.add("hidden");
  setUsbDiscoveryEnabled(true);
  await refreshUsbDeviceList();

  if (usb.webusbSupported) {
    navigator.usb.addEventListener("connect", () => {
      refreshUsbDeviceList();
    });

    navigator.usb.addEventListener("disconnect", async (event) => {
      if (usb.isCurrentDevice(event.device)) {
        await usb.disconnect();
        setUsbDisconnected();
      }
      await refreshUsbDeviceList();
    });
  }

  if (usb.webserialSupported && typeof navigator.serial?.addEventListener === "function") {
    navigator.serial.addEventListener("connect", () => {
      refreshUsbDeviceList();
    });

    navigator.serial.addEventListener("disconnect", async (event) => {
      const port = event?.port || event?.target;
      if (usb.mode === "webserial" && usb.port && port && usb.port === port) {
        await usb.disconnect();
        setUsbDisconnected();
      }
      await refreshUsbDeviceList();
    });
  }
}

function preloadUsbDefaults() {
  if (usbServerUrlInput) {
    const savedServerUrl = window.localStorage.getItem(USB_PREF_SERVER_URL) || "";
    if (savedServerUrl) {
      usbServerUrlInput.value = savedServerUrl;
    } else if (!usbServerUrlInput.value) {
      usbServerUrlInput.value = window.location.origin;
    } else if (usbServerUrlInput.value.includes("lte-swd.example.com")) {
      usbServerUrlInput.value = window.location.origin;
    }
  }

  if (usbEnrollKeyInput) {
    const savedEnrollKey = window.localStorage.getItem(USB_PREF_ENROLL_KEY) || "";
    if (savedEnrollKey) {
      usbEnrollKeyInput.value = savedEnrollKey;
    }
  }
}

function setUsbDiscoveryEnabled(enabled) {
  const nodes = [usbDeviceSelect, usbRefreshBtn, usbPairBtn, usbConnectBtn, usbDisconnectBtn];
  nodes.forEach((node) => {
    if (node) {
      node.disabled = !enabled;
    }
  });
}

function setUsbProvisioningEnabled(enabled) {
  [usbSetForm, usbGetForm].forEach((form) => {
    if (!form) {
      return;
    }
    Array.from(form.elements).forEach((element) => {
      element.disabled = !enabled;
    });
  });
}

function setUsbStatus(color, text) {
  usbState.textContent = text;
  if (color === "green") {
    setLedState(ledUsb, "green", false);
  } else if (color === "yellow") {
    setLedState(ledUsb, "yellow", true);
  } else if (color === "red") {
    setLedState(ledUsb, "red", true);
  } else {
    setLedState(ledUsb, "off", false);
  }
}

function setUsbConnected(productName, mode = "") {
  const modeLabel = mode ? ` (${mode})` : "";
  setUsbProvisioningEnabled(true);
  setUsbStatus("green", `connected${modeLabel}: ${productName}`);
}

function setUsbDisconnected() {
  setUsbProvisioningEnabled(false);
  setUsbStatus("off", "disconnected");
}

async function refreshUsbDeviceList(selectedKey = "") {
  if (!usb.supported) {
    return;
  }

  try {
    const devices = await usb.listKnownDevices();
    renderUsbDeviceList(devices, selectedKey);
  } catch (error) {
    usbConfigView.textContent = String(error.message || error);
  }
}

function renderUsbDeviceList(devices, selectedKey) {
  usbDeviceSelect.innerHTML = "";

  if (!Array.isArray(devices) || devices.length === 0) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "No authorized USB devices";
    usbDeviceSelect.appendChild(option);
    usbConnectBtn.disabled = true;
    return;
  }

  devices.forEach((device) => {
    const option = document.createElement("option");
    option.value = device.key;
    option.textContent = device.connected ? `${device.label} (active)` : device.label;
    usbDeviceSelect.appendChild(option);
  });

  const desiredKey = selectedKey || devices.find((item) => item.connected)?.key || devices[0].key;
  usbDeviceSelect.value = desiredKey;
  updateUsbConnectButtonState();
}

function updateUsbConnectButtonState() {
  const selectedKey = String(usbDeviceSelect.value || "");
  if (!selectedKey) {
    usbConnectBtn.disabled = true;
    return;
  }

  const selectedOption = Array.from(usbDeviceSelect.options).find((option) => option.value === selectedKey);
  const isActive = selectedOption ? selectedOption.textContent.includes("(active)") : false;
  usbConnectBtn.disabled = isActive;
}

async function loadCapabilities() {
  const response = await api.capabilities();
  state.supportedCommands = response.supported_commands || [];

  commandType.innerHTML = "";
  state.supportedCommands.forEach((type) => {
    const option = document.createElement("option");
    option.value = type;
    option.textContent = type;
    commandType.appendChild(option);
  });
}

async function refreshAll() {
  if (state.refreshInFlight) {
    return;
  }

  state.refreshInFlight = true;
  try {
    const response = await api.listDevices();
    state.devices = response.items || [];

    renderDeviceList();
    mapView.updateDevices(state.devices);

    let autoFocused = false;
    if (!state.selectedDeviceId && state.devices.length > 0) {
      state.selectedDeviceId = state.devices[0].device_id;
      autoFocused = true;
    }

    if (state.selectedDeviceId) {
      await refreshSelectedDevice();
      if (autoFocused && state.selectedDevice) {
        mapView.focusDevice(state.selectedDevice);
      }
      await refreshCommands();
    } else {
      state.selectedDevice = null;
      setLedState(ledModem, "off", false);
    }

    connectionState.textContent = "online";
    setLedState(ledCloud, "green", false);
  } catch (error) {
    connectionState.textContent = "degraded";
    commandResult.textContent = `Refresh error: ${String(error.message || error)}`;
    setLedState(ledCloud, "yellow", true);
  } finally {
    state.refreshInFlight = false;
    scheduleRefresh();
  }
}

async function refreshSelectedDevice() {
  const device = await api.getDevice(state.selectedDeviceId);
  state.selectedDevice = device;

  renderDeviceDetail(device);

  if (device.status === "online") {
    setLedState(ledModem, "green", false);
  } else {
    setLedState(ledModem, "red", true);
  }
}

async function refreshCommands() {
  if (!state.selectedDeviceId) {
    return;
  }

  const response = await api.listCommands(state.selectedDeviceId, 50);
  state.commands = response.items || [];
  renderCommandHistory();

  const latest = [...state.commands].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))[0];
  if (!latest) {
    setLedState(ledCommand, "off", false);
    return;
  }

  if (latest.status === "success") {
    setLedState(ledCommand, "green", false);
  } else if (latest.status === "failed") {
    setLedState(ledCommand, "red", true);
  } else {
    setLedState(ledCommand, "yellow", true);
  }
}

function renderDeviceList() {
  deviceList.innerHTML = "";

  if (state.devices.length === 0) {
    const empty = document.createElement("li");
    empty.className = "device-item";
    empty.textContent = "No registered devices";
    deviceList.appendChild(empty);
    return;
  }

  state.devices.forEach((device) => {
    const item = document.createElement("li");
    item.className = `device-item ${device.device_id === state.selectedDeviceId ? "active" : ""}`;

    const button = document.createElement("button");
    button.type = "button";
    button.textContent = `${device.device_id} | ${device.status}`;
    button.addEventListener("click", async () => {
      state.selectedDeviceId = device.device_id;
      renderDeviceList();
      await refreshSelectedDevice();
      if (state.selectedDevice) {
        mapView.focusDevice(state.selectedDevice);
      }
      await refreshCommands();
    });

    item.appendChild(button);
    deviceList.appendChild(item);
  });
}

function scheduleRefresh() {
  if (state.refreshTimer) {
    clearTimeout(state.refreshTimer);
  }
  state.refreshTimer = setTimeout(() => {
    refreshAll();
  }, 5000);
}

function escapeHtml(str) {
  return String(str)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function renderDeviceDetail(device) {
  const rssiValue = Number(device.last_telemetry?.rssi_dbm);
  const latValue = Number(device.last_location?.lat);
  const lonValue = Number(device.last_location?.lon);
  const coordinates =
    Number.isFinite(latValue) && Number.isFinite(lonValue)
      ? `${latValue.toFixed(6)}, ${lonValue.toFixed(6)}`
      : "n/a";

  const fields = [
    ["device_id", device.device_id],
    ["status", device.status],
    ["firmware", device.firmware_version || "n/a"],
    ["imei", device.modem_imei || "n/a"],
    ["sim_iccid", device.sim_iccid || "n/a"],
    ["last_seen", formatTimestamp(device.last_seen_at)],
    ["rssi_dbm", formatSignalReadout(rssiValue)],
    ["network", device.last_telemetry?.network_state ?? "n/a"],
    ["coordinates", coordinates],
    ["accuracy_m", device.last_location?.accuracy_m ?? "n/a"],
  ];

  deviceDetail.innerHTML = "";
  fields.forEach(([label, value]) => {
    const row = document.createElement("div");
    row.className = "detail-line";
    // formatSignalReadout returns trusted HTML; all other values are escaped
    const safeValue = typeof value === "string" && !value.includes('<span') ? escapeHtml(value) : value;
    row.innerHTML = `<span class="detail-label">${escapeHtml(label)}</span><span class="detail-value">${safeValue}</span>`;
    deviceDetail.appendChild(row);
  });
}

function formatSignalReadout(rssi) {
  if (!Number.isFinite(rssi)) {
    return "n/a";
  }

  const bars = rssiToBars(rssi);
  const barsHtml = [1, 2, 3, 4, 5]
    .map((level) => `<span class="signal-bar${bars >= level ? " active" : ""}"></span>`)
    .join("");

  return `<span class="signal-readout"><span class="signal-value">${rssi}</span><span class="signal-meter" aria-label="Signal level">${barsHtml}</span></span>`;
}

function rssiToBars(rssi) {
  if (rssi >= -70) return 5;
  if (rssi >= -80) return 4;
  if (rssi >= -90) return 3;
  if (rssi >= -100) return 2;
  if (rssi >= -110) return 1;
  return 0;
}

function renderCommandHistory() {
  commandHistory.innerHTML = "";

  if (state.commands.length === 0) {
    const empty = document.createElement("li");
    empty.className = "history-item";
    empty.textContent = "Command history is empty";
    commandHistory.appendChild(empty);
    return;
  }

  [...state.commands]
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))
    .forEach((command) => {
      const item = document.createElement("li");
      item.className = "history-item";
      const payload = safeJSONStringify(command.payload);
      const result = safeJSONStringify(command.result);

      item.innerHTML = [
        `<strong>${command.type}</strong> (${command.status})`,
        `<div class="muted">id: ${command.command_id}</div>`,
        `<div class="muted">created: ${formatTimestamp(command.created_at)}</div>`,
        `<div class="muted">payload: ${payload}</div>`,
        `<div class="muted">result: ${result}</div>`,
      ].join("");
      commandHistory.appendChild(item);
    });
}

function setLedState(node, color, pulse) {
  if (!node) {
    return;
  }

  node.classList.remove("led-on-green", "led-on-yellow", "led-on-red", "led-pulse");

  if (color === "green") {
    node.classList.add("led-on-green");
  } else if (color === "yellow") {
    node.classList.add("led-on-yellow");
  } else if (color === "red") {
    node.classList.add("led-on-red");
  }

  if (pulse) {
    node.classList.add("led-pulse");
  }
}

function formatTimestamp(value) {
  if (!value) {
    return "n/a";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return date.toLocaleString();
}

function safeJSONStringify(value) {
  if (value === undefined || value === null) {
    return "n/a";
  }
  if (typeof value === "string") {
    return value;
  }
  try {
    return JSON.stringify(value);
  } catch {
    return "[unserializable]";
  }
}