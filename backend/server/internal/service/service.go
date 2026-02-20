package service

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"lte_swd/backend/server/internal/auth"
	"lte_swd/backend/server/internal/config"
	"lte_swd/backend/server/internal/model"
	"lte_swd/backend/server/internal/store"
)

var supportedCommandTypes = map[string]struct{}{
	"swd_connect":       {},
	"swd_read_memory":   {},
	"swd_write_memory":  {},
	"swd_erase":         {},
	"swd_program":       {},
	"swd_verify":        {},
	"swd_copy_firmware": {},
	"swd_reset":         {},
}

// Service contains business rules for LTE_SWD R1 backend.
type Service struct {
	cfg   config.Config
	store *store.StateStore
	auth  *auth.OperatorAuth
	nowFn func() time.Time
}

// New creates service layer over auth and state store.
func New(cfg config.Config, st *store.StateStore, opAuth *auth.OperatorAuth) *Service {
	return &Service{
		cfg:   cfg,
		store: st,
		auth:  opAuth,
		nowFn: time.Now,
	}
}

// LoginOperator validates password and returns bearer token.
func (s *Service) LoginOperator(password string) (string, time.Time, error) {
	return s.auth.Login(strings.TrimSpace(password), s.nowFn().UTC())
}

// RequireOperator checks bearer token.
func (s *Service) RequireOperator(token string) error {
	return s.auth.Validate(token, s.nowFn().UTC())
}

// RegisterDeviceRequest describes first registration payload.
type RegisterDeviceRequest struct {
	EnrollKey       string `json:"enroll_key"`
	DeviceID        string `json:"device_id"`
	HWUID           string `json:"hw_uid"`
	ModemIMEI       string `json:"modem_imei"`
	SimICCID        string `json:"sim_iccid"`
	FirmwareVersion string `json:"firmware_version"`
}

// RegisterDeviceResponse includes issued token and poll timing.
type RegisterDeviceResponse struct {
	DeviceToken          string `json:"device_token"`
	PollIntervalSec      int    `json:"poll_interval_sec"`
	HeartbeatIntervalSec int    `json:"heartbeat_interval_sec"`
}

// RegisterDevice performs enrollment validation and registration.
func (s *Service) RegisterDevice(req RegisterDeviceRequest) (RegisterDeviceResponse, error) {
	if subtle.ConstantTimeCompare([]byte(req.EnrollKey), []byte(s.cfg.DeviceEnrollKey)) != 1 {
		return RegisterDeviceResponse{}, errors.New("invalid enroll key")
	}

	req.DeviceID = strings.TrimSpace(req.DeviceID)
	if req.DeviceID == "" {
		return RegisterDeviceResponse{}, errors.New("device_id is required")
	}

	device, _, err := s.store.RegisterDevice(
		req.DeviceID,
		strings.TrimSpace(req.HWUID),
		strings.TrimSpace(req.ModemIMEI),
		strings.TrimSpace(req.SimICCID),
		strings.TrimSpace(req.FirmwareVersion),
		s.nowFn().UTC(),
	)
	if err != nil {
		return RegisterDeviceResponse{}, err
	}

	return RegisterDeviceResponse{
		DeviceToken:          device.DeviceToken,
		PollIntervalSec:      3,
		HeartbeatIntervalSec: 10,
	}, nil
}

// DeviceAuthRequest keeps token validation data.
type DeviceAuthRequest struct {
	DeviceID    string `json:"device_id"`
	DeviceToken string `json:"device_token"`
}

// DeviceHeartbeat validates device and updates heartbeat.
func (s *Service) DeviceHeartbeat(req DeviceAuthRequest) error {
	return s.store.AddHeartbeat(strings.TrimSpace(req.DeviceID), strings.TrimSpace(req.DeviceToken), s.nowFn().UTC())
}

// DeviceTelemetryRequest describes telemetry push payload.
type DeviceTelemetryRequest struct {
	DeviceID    string          `json:"device_id"`
	DeviceToken string          `json:"device_token"`
	Data        model.Telemetry `json:"data"`
}

// DeviceTelemetry stores telemetry sample.
func (s *Service) DeviceTelemetry(req DeviceTelemetryRequest) error {
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.DeviceToken = strings.TrimSpace(req.DeviceToken)
	if req.DeviceID == "" || req.DeviceToken == "" {
		return errors.New("device_id and device_token are required")
	}
	return s.store.AddTelemetry(req.DeviceID, req.DeviceToken, req.Data, s.nowFn().UTC())
}

// DeviceLocationRequest describes location push payload.
type DeviceLocationRequest struct {
	DeviceID    string         `json:"device_id"`
	DeviceToken string         `json:"device_token"`
	Data        model.Location `json:"data"`
}

// DeviceLocation stores location sample.
func (s *Service) DeviceLocation(req DeviceLocationRequest) error {
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.DeviceToken = strings.TrimSpace(req.DeviceToken)
	if req.DeviceID == "" || req.DeviceToken == "" {
		return errors.New("device_id and device_token are required")
	}
	return s.store.AddLocation(req.DeviceID, req.DeviceToken, req.Data, s.nowFn().UTC())
}

// DevicePullRequest contains command pull auth payload.
type DevicePullRequest struct {
	DeviceID    string `json:"device_id"`
	DeviceToken string `json:"device_token"`
}

// DevicePullCommand returns next queued command for device.
func (s *Service) DevicePullCommand(req DevicePullRequest) (*model.Command, error) {
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.DeviceToken = strings.TrimSpace(req.DeviceToken)
	if req.DeviceID == "" || req.DeviceToken == "" {
		return nil, errors.New("device_id and device_token are required")
	}
	return s.store.PullNextCommand(req.DeviceID, req.DeviceToken, s.nowFn().UTC())
}

// DeviceCommandResultRequest describes command completion payload.
type DeviceCommandResultRequest struct {
	DeviceID    string                 `json:"device_id"`
	DeviceToken string                 `json:"device_token"`
	CommandID   string                 `json:"command_id"`
	Status      model.CommandStatus    `json:"status"`
	Message     string                 `json:"message"`
	Metrics     map[string]interface{} `json:"metrics"`
	Data        map[string]interface{} `json:"data"`
}

// DeviceCommandResult stores command completion.
func (s *Service) DeviceCommandResult(req DeviceCommandResultRequest) (*model.Command, error) {
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.DeviceToken = strings.TrimSpace(req.DeviceToken)
	req.CommandID = strings.TrimSpace(req.CommandID)

	if req.DeviceID == "" || req.DeviceToken == "" || req.CommandID == "" {
		return nil, errors.New("device_id, device_token and command_id are required")
	}

	resultStatus := req.Status
	if resultStatus != model.CommandSuccess {
		resultStatus = model.CommandFailed
	}

	return s.store.CompleteCommand(req.DeviceID, req.DeviceToken, req.CommandID, model.CommandResult{
		Status:  resultStatus,
		Message: req.Message,
		Metrics: req.Metrics,
		Data:    req.Data,
	}, s.nowFn().UTC())
}

// DeviceGetArtifact validates device token and returns artifact.
func (s *Service) DeviceGetArtifact(deviceID, deviceToken, artifactID string) (*model.Artifact, error) {
	deviceID = strings.TrimSpace(deviceID)
	deviceToken = strings.TrimSpace(deviceToken)
	artifactID = strings.TrimSpace(artifactID)

	if deviceID == "" || deviceToken == "" || artifactID == "" {
		return nil, errors.New("device_id, device_token and artifact_id are required")
	}

	if _, err := s.store.ValidateDeviceToken(deviceID, deviceToken, s.nowFn().UTC()); err != nil {
		return nil, err
	}
	return s.store.GetArtifact(artifactID)
}

// OperatorCommandRequest describes operator command payload.
type OperatorCommandRequest struct {
	DeviceID string          `json:"device_id"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
}

// OperatorCreateCommand enqueues new command for one device.
func (s *Service) OperatorCreateCommand(req OperatorCommandRequest, operator string) (*model.Command, error) {
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.Type = strings.TrimSpace(req.Type)

	if req.DeviceID == "" || req.Type == "" {
		return nil, errors.New("device_id and type are required")
	}
	if _, ok := supportedCommandTypes[req.Type]; !ok {
		return nil, fmt.Errorf("unsupported command type: %s", req.Type)
	}

	if len(req.Payload) == 0 {
		req.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(req.Payload) {
		return nil, errors.New("payload must be valid json")
	}

	return s.store.AddCommand(req.DeviceID, req.Type, req.Payload, operator, s.nowFn().UTC())
}

// OperatorArtifactRequest describes uploaded firmware payload.
type OperatorArtifactRequest struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Base64Data  string `json:"base64_data"`
}

// OperatorUploadArtifact stores firmware artifact for swd_program operations.
func (s *Service) OperatorUploadArtifact(req OperatorArtifactRequest, operator string) (*model.Artifact, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.ContentType = strings.TrimSpace(req.ContentType)
	req.Base64Data = strings.TrimSpace(req.Base64Data)

	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if req.Base64Data == "" {
		return nil, errors.New("base64_data is required")
	}

	data, err := base64.StdEncoding.DecodeString(req.Base64Data)
	if err != nil {
		return nil, errors.New("base64_data must be valid base64")
	}
	if len(data) == 0 {
		return nil, errors.New("artifact payload must not be empty")
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return s.store.SaveArtifact(req.Name, contentType, data, operator, s.nowFn().UTC())
}

// OperatorGetArtifact returns stored artifact.
func (s *Service) OperatorGetArtifact(artifactID string) (*model.Artifact, error) {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return nil, errors.New("artifact_id is required")
	}
	return s.store.GetArtifact(artifactID)
}

// OperatorListDevices returns fleet state.
func (s *Service) OperatorListDevices() ([]*model.Device, error) {
	return s.store.ListDevices(s.nowFn().UTC(), s.cfg.DeviceOfflineAfter)
}

// OperatorGetDevice returns one device snapshot.
func (s *Service) OperatorGetDevice(deviceID string) (*model.Device, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, errors.New("device_id is required")
	}
	return s.store.GetDevice(deviceID, s.nowFn().UTC(), s.cfg.DeviceOfflineAfter)
}

// OperatorListTelemetry returns telemetry history.
func (s *Service) OperatorListTelemetry(deviceID string, limit int) ([]model.TelemetryRecord, error) {
	return s.store.ListTelemetry(strings.TrimSpace(deviceID), limit)
}

// OperatorListCommands returns command history.
func (s *Service) OperatorListCommands(deviceID string, limit int) ([]*model.Command, error) {
	return s.store.ListCommands(strings.TrimSpace(deviceID), limit)
}

// SupportedCommandTypes returns deterministic command type list.
func SupportedCommandTypes() []string {
	keys := make([]string, 0, len(supportedCommandTypes))
	for key := range supportedCommandTypes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
