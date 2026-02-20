package model

import (
	"encoding/json"
	"time"
)

// DeviceStatus defines runtime connectivity state.
type DeviceStatus string

const (
	// DeviceStatusOnline marks a device as reachable by API heartbeat.
	DeviceStatusOnline DeviceStatus = "online"
	// DeviceStatusOffline marks a device as stale.
	DeviceStatusOffline DeviceStatus = "offline"
)

// CommandStatus defines lifecycle of operator command.
type CommandStatus string

const (
	// CommandQueued means backend accepted command.
	CommandQueued CommandStatus = "queued"
	// CommandDispatched means command was delivered to device.
	CommandDispatched CommandStatus = "dispatched"
	// CommandSuccess means execution finished successfully.
	CommandSuccess CommandStatus = "success"
	// CommandFailed means execution ended with error.
	CommandFailed CommandStatus = "failed"
)

// Device keeps metadata and last known state.
type Device struct {
	DeviceID        string       `json:"device_id"`
	HWUID           string       `json:"hw_uid"`
	ModemIMEI       string       `json:"modem_imei"`
	SimICCID        string       `json:"sim_iccid"`
	FirmwareVersion string       `json:"firmware_version"`
	DeviceToken     string       `json:"device_token"`
	RegisteredAt    time.Time    `json:"registered_at"`
	LastSeenAt      time.Time    `json:"last_seen_at"`
	LastHeartbeatAt time.Time    `json:"last_heartbeat_at"`
	LastTelemetryAt time.Time    `json:"last_telemetry_at"`
	LastLocationAt  time.Time    `json:"last_location_at"`
	LastTelemetry   *Telemetry   `json:"last_telemetry,omitempty"`
	LastLocation    *Location    `json:"last_location,omitempty"`
	Status          DeviceStatus `json:"status"`
}

// Telemetry stores periodic device metrics.
type Telemetry struct {
	BatteryMV    int                    `json:"battery_mv"`
	SupplyMV     int                    `json:"supply_mv"`
	TemperatureC float64                `json:"temperature_c"`
	RSSIDBM      int                    `json:"rssi_dbm"`
	NetworkState string                 `json:"network_state"`
	UptimeSec    uint64                 `json:"uptime_sec"`
	Extra        map[string]interface{} `json:"extra,omitempty"`
}

// Location stores last known coordinates.
type Location struct {
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	AltM      float64 `json:"alt_m"`
	AccuracyM float64 `json:"accuracy_m"`
	Source    string  `json:"source"`
}

// TelemetryRecord stores immutable telemetry history point.
type TelemetryRecord struct {
	DeviceID  string    `json:"device_id"`
	Timestamp time.Time `json:"timestamp"`
	Data      Telemetry `json:"data"`
}

// CommandResult stores the device execution output.
type CommandResult struct {
	Status  CommandStatus          `json:"status"`
	Message string                 `json:"message"`
	Metrics map[string]interface{} `json:"metrics,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// Command stores a queued SWD action.
type Command struct {
	CommandID    string          `json:"command_id"`
	DeviceID     string          `json:"device_id"`
	Type         string          `json:"type"`
	Payload      json.RawMessage `json:"payload"`
	CreatedBy    string          `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
	DispatchedAt *time.Time      `json:"dispatched_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	Status       CommandStatus   `json:"status"`
	Result       *CommandResult  `json:"result,omitempty"`
}

// Artifact stores binary payload for program/copy operations.
type Artifact struct {
	ArtifactID    string    `json:"artifact_id"`
	Name          string    `json:"name"`
	ContentType   string    `json:"content_type"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	Payload       []byte    `json:"payload"`
	PayloadSHA256 string    `json:"payload_sha256"`
}

// PersistedState keeps whole R1 server state snapshot.
type PersistedState struct {
	Devices       map[string]*Device           `json:"devices"`
	TelemetryByID map[string][]TelemetryRecord `json:"telemetry_by_id"`
	CommandsByID  map[string][]*Command        `json:"commands_by_id"`
	Artifacts     map[string]*Artifact         `json:"artifacts"`
}

// CloneDevice creates copy that caller can mutate safely.
func CloneDevice(src *Device) *Device {
	if src == nil {
		return nil
	}
	out := *src
	if src.LastTelemetry != nil {
		telemetry := *src.LastTelemetry
		if telemetry.Extra != nil {
			telemetry.Extra = cloneMap(src.LastTelemetry.Extra)
		}
		out.LastTelemetry = &telemetry
	}
	if src.LastLocation != nil {
		location := *src.LastLocation
		out.LastLocation = &location
	}
	return &out
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
