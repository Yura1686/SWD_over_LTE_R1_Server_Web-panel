package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"lte_swd/backend/server/internal/model"
	"lte_swd/backend/server/internal/util"
)

const maxTelemetryHistory = 500

// StateStore keeps R1 runtime state with JSON file persistence.
type StateStore struct {
	mu         sync.RWMutex
	fleetLimit int
	dataFile   string
	state      model.PersistedState
}

// NewStateStore creates state store and loads prior snapshot when available.
func NewStateStore(dataFile string, fleetLimit int) (*StateStore, error) {
	s := &StateStore{
		fleetLimit: fleetLimit,
		dataFile:   dataFile,
		state: model.PersistedState{
			Devices:       make(map[string]*model.Device),
			TelemetryByID: make(map[string][]model.TelemetryRecord),
			CommandsByID:  make(map[string][]*model.Command),
			Artifacts:     make(map[string]*model.Artifact),
		},
	}

	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *StateStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read state file: %w", err)
	}

	var loaded model.PersistedState
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("unmarshal state: %w", err)
	}

	if loaded.Devices == nil {
		loaded.Devices = make(map[string]*model.Device)
	}
	if loaded.TelemetryByID == nil {
		loaded.TelemetryByID = make(map[string][]model.TelemetryRecord)
	}
	if loaded.CommandsByID == nil {
		loaded.CommandsByID = make(map[string][]*model.Command)
	}
	if loaded.Artifacts == nil {
		loaded.Artifacts = make(map[string]*model.Artifact)
	}

	s.state = loaded
	return nil
}

func (s *StateStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.dataFile), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tempFile := s.dataFile + ".tmp"
	if err := os.WriteFile(tempFile, raw, 0o644); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}

	if err := os.Rename(tempFile, s.dataFile); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}

// RegisterDevice creates or refreshes a device record and returns token.
func (s *StateStore) RegisterDevice(deviceID, hwUID, modemIMEI, simICCID, firmwareVersion string, now time.Time) (*model.Device, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.state.Devices[deviceID]; ok {
		if (existing.HWUID != "" && hwUID != "" && existing.HWUID != hwUID) ||
			(existing.ModemIMEI != "" && modemIMEI != "" && existing.ModemIMEI != modemIMEI) {
			return nil, false, ErrDeviceExistsWithOtherIdentity
		}

		existing.HWUID = firstNonEmpty(existing.HWUID, hwUID)
		existing.ModemIMEI = firstNonEmpty(existing.ModemIMEI, modemIMEI)
		existing.SimICCID = firstNonEmpty(existing.SimICCID, simICCID)
		existing.FirmwareVersion = firstNonEmpty(firmwareVersion, existing.FirmwareVersion)
		existing.LastSeenAt = now
		existing.LastHeartbeatAt = now
		existing.Status = model.DeviceStatusOnline

		if err := s.persistLocked(); err != nil {
			return nil, false, err
		}
		return model.CloneDevice(existing), false, nil
	}

	if len(s.state.Devices) >= s.fleetLimit {
		return nil, false, ErrFleetLimitReached
	}

	created := &model.Device{
		DeviceID:        deviceID,
		HWUID:           hwUID,
		ModemIMEI:       modemIMEI,
		SimICCID:        simICCID,
		FirmwareVersion: firmwareVersion,
		DeviceToken:     util.RandomToken("dev", 16),
		RegisteredAt:    now,
		LastSeenAt:      now,
		LastHeartbeatAt: now,
		Status:          model.DeviceStatusOnline,
	}

	s.state.Devices[deviceID] = created
	if err := s.persistLocked(); err != nil {
		return nil, false, err
	}
	return model.CloneDevice(created), true, nil
}

// ValidateDeviceToken checks that device exists and token matches.
func (s *StateStore) ValidateDeviceToken(deviceID, deviceToken string, now time.Time) (*model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, ok := s.state.Devices[deviceID]
	if !ok {
		return nil, ErrDeviceNotFound
	}
	if device.DeviceToken != deviceToken {
		return nil, ErrInvalidDeviceToken
	}

	device.LastSeenAt = now
	device.Status = model.DeviceStatusOnline
	if err := s.persistLocked(); err != nil {
		return nil, err
	}

	return model.CloneDevice(device), nil
}

// AddHeartbeat updates connectivity timestamp for active device.
func (s *StateStore) AddHeartbeat(deviceID, deviceToken string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, err := s.requireDeviceLocked(deviceID, deviceToken)
	if err != nil {
		return err
	}

	device.LastSeenAt = now
	device.LastHeartbeatAt = now
	device.Status = model.DeviceStatusOnline
	return s.persistLocked()
}

// AddTelemetry appends telemetry history and updates device snapshot.
func (s *StateStore) AddTelemetry(deviceID, deviceToken string, data model.Telemetry, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, err := s.requireDeviceLocked(deviceID, deviceToken)
	if err != nil {
		return err
	}

	record := model.TelemetryRecord{
		DeviceID:  deviceID,
		Timestamp: now,
		Data:      data,
	}

	list := append(s.state.TelemetryByID[deviceID], record)
	if len(list) > maxTelemetryHistory {
		list = list[len(list)-maxTelemetryHistory:]
	}
	s.state.TelemetryByID[deviceID] = list

	copyTelemetry := data
	if copyTelemetry.Extra != nil {
		copied := make(map[string]interface{}, len(copyTelemetry.Extra))
		for k, v := range copyTelemetry.Extra {
			copied[k] = v
		}
		copyTelemetry.Extra = copied
	}

	device.LastTelemetry = &copyTelemetry
	device.LastTelemetryAt = now
	device.LastSeenAt = now
	device.Status = model.DeviceStatusOnline
	return s.persistLocked()
}

// AddLocation updates latest coordinates for a device.
func (s *StateStore) AddLocation(deviceID, deviceToken string, location model.Location, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, err := s.requireDeviceLocked(deviceID, deviceToken)
	if err != nil {
		return err
	}

	copyLocation := location
	device.LastLocation = &copyLocation
	device.LastLocationAt = now
	device.LastSeenAt = now
	device.Status = model.DeviceStatusOnline
	return s.persistLocked()
}

// ListDevices returns sorted device list with refreshed online/offline status.
func (s *StateStore) ListDevices(now time.Time, offlineAfter time.Duration) ([]*model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*model.Device, 0, len(s.state.Devices))
	for _, device := range s.state.Devices {
		if now.Sub(device.LastSeenAt) > offlineAfter {
			device.Status = model.DeviceStatusOffline
		} else {
			device.Status = model.DeviceStatusOnline
		}
		out = append(out, model.CloneDevice(device))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].DeviceID < out[j].DeviceID
	})

	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetDevice returns one device with status refresh.
func (s *StateStore) GetDevice(deviceID string, now time.Time, offlineAfter time.Duration) (*model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, ok := s.state.Devices[deviceID]
	if !ok {
		return nil, ErrDeviceNotFound
	}

	if now.Sub(device.LastSeenAt) > offlineAfter {
		device.Status = model.DeviceStatusOffline
	} else {
		device.Status = model.DeviceStatusOnline
	}

	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return model.CloneDevice(device), nil
}

// ListTelemetry returns the latest telemetry records for device.
func (s *StateStore) ListTelemetry(deviceID string, limit int) ([]model.TelemetryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.state.Devices[deviceID]; !ok {
		return nil, ErrDeviceNotFound
	}

	items := s.state.TelemetryByID[deviceID]
	if limit <= 0 || limit >= len(items) {
		return append([]model.TelemetryRecord(nil), items...), nil
	}

	start := len(items) - limit
	return append([]model.TelemetryRecord(nil), items[start:]...), nil
}

// AddCommand pushes new command to the selected device queue.
func (s *StateStore) AddCommand(deviceID, commandType string, payload []byte, createdBy string, now time.Time) (*model.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.state.Devices[deviceID]; !ok {
		return nil, ErrDeviceNotFound
	}

	command := &model.Command{
		CommandID: util.RandomToken("cmd", 12),
		DeviceID:  deviceID,
		Type:      commandType,
		Payload:   append([]byte(nil), payload...),
		CreatedBy: createdBy,
		CreatedAt: now,
		Status:    model.CommandQueued,
	}

	s.state.CommandsByID[deviceID] = append(s.state.CommandsByID[deviceID], command)
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return cloneCommand(command), nil
}

// ListCommands returns command history for a device.
func (s *StateStore) ListCommands(deviceID string, limit int) ([]*model.Command, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.state.Devices[deviceID]; !ok {
		return nil, ErrDeviceNotFound
	}

	items := s.state.CommandsByID[deviceID]
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}

	out := make([]*model.Command, 0, len(items))
	for _, item := range items {
		out = append(out, cloneCommand(item))
	}
	return out, nil
}

// PullNextCommand dispatches first queued command for device.
func (s *StateStore) PullNextCommand(deviceID, deviceToken string, now time.Time) (*model.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, err := s.requireDeviceLocked(deviceID, deviceToken)
	if err != nil {
		return nil, err
	}

	queue := s.state.CommandsByID[deviceID]
	for _, item := range queue {
		if item.Status == model.CommandQueued {
			item.Status = model.CommandDispatched
			dispatchTime := now
			item.DispatchedAt = &dispatchTime
			device.LastSeenAt = now
			device.Status = model.DeviceStatusOnline
			if err := s.persistLocked(); err != nil {
				return nil, err
			}
			return cloneCommand(item), nil
		}
	}

	device.LastSeenAt = now
	device.Status = model.DeviceStatusOnline
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return nil, nil
}

// CompleteCommand stores final result for one dispatched command.
func (s *StateStore) CompleteCommand(deviceID, deviceToken, commandID string, result model.CommandResult, now time.Time) (*model.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, err := s.requireDeviceLocked(deviceID, deviceToken)
	if err != nil {
		return nil, err
	}

	queue := s.state.CommandsByID[deviceID]
	for _, item := range queue {
		if item.CommandID != commandID {
			continue
		}

		if result.Status == "" {
			result.Status = model.CommandFailed
		}

		completedAt := now
		item.CompletedAt = &completedAt
		item.Result = &result
		if result.Status == model.CommandSuccess {
			item.Status = model.CommandSuccess
		} else {
			item.Status = model.CommandFailed
		}

		device.LastSeenAt = now
		device.Status = model.DeviceStatusOnline

		if err := s.persistLocked(); err != nil {
			return nil, err
		}
		return cloneCommand(item), nil
	}

	return nil, ErrCommandNotFound
}

// SaveArtifact stores binary payload and returns artifact metadata.
func (s *StateStore) SaveArtifact(name, contentType string, payload []byte, createdBy string, now time.Time) (*model.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	digest := sha256.Sum256(payload)
	digestHex := hex.EncodeToString(digest[:])
	artifactID := "art_" + digestHex[:24]

	if existing, ok := s.state.Artifacts[artifactID]; ok {
		return cloneArtifact(existing), nil
	}

	artifact := &model.Artifact{
		ArtifactID:    artifactID,
		Name:          name,
		ContentType:   contentType,
		CreatedBy:     createdBy,
		CreatedAt:     now,
		Payload:       append([]byte(nil), payload...),
		PayloadSHA256: digestHex,
	}

	s.state.Artifacts[artifactID] = artifact
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return cloneArtifact(artifact), nil
}

// GetArtifact returns artifact metadata and payload.
func (s *StateStore) GetArtifact(artifactID string) (*model.Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	artifact, ok := s.state.Artifacts[artifactID]
	if !ok {
		return nil, ErrArtifactNotFound
	}
	return cloneArtifact(artifact), nil
}

// DeviceCount returns registered devices count.
func (s *StateStore) DeviceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.state.Devices)
}

func (s *StateStore) requireDeviceLocked(deviceID, token string) (*model.Device, error) {
	device, ok := s.state.Devices[deviceID]
	if !ok {
		return nil, ErrDeviceNotFound
	}
	if device.DeviceToken != token {
		return nil, ErrInvalidDeviceToken
	}
	return device, nil
}

func cloneCommand(src *model.Command) *model.Command {
	if src == nil {
		return nil
	}
	out := *src
	if src.Payload != nil {
		out.Payload = append([]byte(nil), src.Payload...)
	}
	if src.Result != nil {
		result := *src.Result
		result.Metrics = cloneStringAny(src.Result.Metrics)
		result.Data = cloneStringAny(src.Result.Data)
		out.Result = &result
	}
	if src.DispatchedAt != nil {
		ts := *src.DispatchedAt
		out.DispatchedAt = &ts
	}
	if src.CompletedAt != nil {
		ts := *src.CompletedAt
		out.CompletedAt = &ts
	}
	return &out
}

func cloneArtifact(src *model.Artifact) *model.Artifact {
	if src == nil {
		return nil
	}
	out := *src
	if src.Payload != nil {
		out.Payload = append([]byte(nil), src.Payload...)
	}
	return &out
}

func cloneStringAny(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func firstNonEmpty(primary, secondary string) string {
	if primary != "" {
		return primary
	}
	return secondary
}
