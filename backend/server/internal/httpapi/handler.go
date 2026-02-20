package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lte_swd/backend/server/internal/auth"
	"lte_swd/backend/server/internal/service"
	"lte_swd/backend/server/internal/store"
)

// Handler exposes HTTP API and static frontend for R1.
type Handler struct {
	svc               *service.Service
	staticDir         string
	maxJSONBytes      int64
	maxArtifactBytes  int64
	apiRateLimiter    *ipRateLimiter
	loginGuard        *loginGuard
	trustProxyHeaders bool
}

// Options contains HTTP API and security parameters.
type Options struct {
	MaxJSONBytes      int64
	MaxArtifactBytes  int64
	APIRatePerMinute  int
	LoginRatePerMin   int
	LoginBurst        int
	TrustProxyHeaders bool
}

// NewHandler creates API handler.
func NewHandler(svc *service.Service, staticDir string, options Options) *Handler {
	maxJSONBytes := options.MaxJSONBytes
	if maxJSONBytes <= 0 {
		maxJSONBytes = 64 * 1024
	}

	maxArtifactBytes := options.MaxArtifactBytes
	if maxArtifactBytes < maxJSONBytes {
		maxArtifactBytes = 12 * 1024 * 1024
	}

	return &Handler{
		svc:               svc,
		staticDir:         staticDir,
		maxJSONBytes:      maxJSONBytes,
		maxArtifactBytes:  maxArtifactBytes,
		apiRateLimiter:    newIPRateLimiter(options.APIRatePerMinute, time.Minute),
		loginGuard:        newLoginGuard(options.LoginRatePerMin, options.LoginBurst),
		trustProxyHeaders: options.TrustProxyHeaders,
	}
}

// BuildMux wires API routes and static assets.
func (h *Handler) BuildMux() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/operator/login", h.handleOperatorLogin)
	mux.HandleFunc("GET /api/v1/operator/capabilities", h.requireOperator(h.handleOperatorCapabilities))

	mux.HandleFunc("GET /api/v1/devices", h.requireOperator(h.handleListDevices))
	mux.HandleFunc("GET /api/v1/devices/{device_id}", h.requireOperator(h.handleGetDevice))
	mux.HandleFunc("GET /api/v1/devices/{device_id}/telemetry", h.requireOperator(h.handleListTelemetry))
	mux.HandleFunc("GET /api/v1/devices/{device_id}/commands", h.requireOperator(h.handleListCommands))
	mux.HandleFunc("POST /api/v1/commands", h.requireOperator(h.handleCreateCommand))
	mux.HandleFunc("POST /api/v1/artifacts", h.requireOperator(h.handleUploadArtifact))
	mux.HandleFunc("GET /api/v1/artifacts/{artifact_id}", h.requireOperator(h.handleGetArtifact))

	mux.HandleFunc("POST /api/v1/device/register", h.handleDeviceRegister)
	mux.HandleFunc("POST /api/v1/device/heartbeat", h.handleDeviceHeartbeat)
	mux.HandleFunc("POST /api/v1/device/telemetry", h.handleDeviceTelemetry)
	mux.HandleFunc("POST /api/v1/device/location", h.handleDeviceLocation)
	mux.HandleFunc("POST /api/v1/device/commands/pull", h.handleDevicePullCommand)
	mux.HandleFunc("POST /api/v1/device/commands/{command_id}/result", h.handleDeviceCommandResult)
	mux.HandleFunc("GET /api/v1/device/artifacts/{artifact_id}", h.handleDeviceGetArtifact)

	staticRoot, _ := filepath.Abs(h.staticDir)
	fs := http.FileServer(http.Dir(staticRoot))
	mux.Handle("/", fs)

	return h.withSecurityHeaders(h.withRateLimit(h.withLogging(mux)))
}

func (h *Handler) handleOperatorLogin(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	ip := requestIP(r, h.trustProxyHeaders)
	allowed, retryAfter := h.loginGuard.allow(ip, now)
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
		writeError(w, http.StatusTooManyRequests, errors.New("too many login attempts, try later"))
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req, h.maxJSONBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	token, expiresAt, err := h.svc.LoginOperator(req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidPassword) {
			h.loginGuard.onFailure(ip, now)
		}
		writeErrorFromDomain(w, err)
		return
	}
	h.loginGuard.onSuccess(ip)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"expires_at": expiresAt,
	})
}

func (h *Handler) handleOperatorCapabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"supported_commands": service.SupportedCommandTypes(),
	})
}

func (h *Handler) handleListDevices(w http.ResponseWriter, _ *http.Request) {
	devices, err := h.svc.OperatorListDevices()
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": devices})
}

func (h *Handler) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	device, err := h.svc.OperatorGetDevice(deviceID)
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}
	writeJSON(w, http.StatusOK, device)
}

func (h *Handler) handleListTelemetry(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	limit := parseIntOrDefault(r.URL.Query().Get("limit"), 100)

	telemetry, err := h.svc.OperatorListTelemetry(deviceID, limit)
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": telemetry})
}

func (h *Handler) handleListCommands(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	limit := parseIntOrDefault(r.URL.Query().Get("limit"), 100)

	commands, err := h.svc.OperatorListCommands(deviceID, limit)
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": commands})
}

func (h *Handler) handleCreateCommand(w http.ResponseWriter, r *http.Request) {
	var req service.OperatorCommandRequest
	if err := decodeJSON(r, &req, h.maxJSONBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	command, err := h.svc.OperatorCreateCommand(req, operatorFromRequest(r))
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, command)
}

func (h *Handler) handleUploadArtifact(w http.ResponseWriter, r *http.Request) {
	var req service.OperatorArtifactRequest
	if err := decodeJSON(r, &req, h.maxArtifactBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	artifact, err := h.svc.OperatorUploadArtifact(req, operatorFromRequest(r))
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"artifact_id":    artifact.ArtifactID,
		"name":           artifact.Name,
		"content_type":   artifact.ContentType,
		"size":           len(artifact.Payload),
		"payload_sha256": artifact.PayloadSHA256,
	})
}

func (h *Handler) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	artifactID := r.PathValue("artifact_id")
	artifact, err := h.svc.OperatorGetArtifact(artifactID)
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}

	w.Header().Set("Content-Type", artifact.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifact.Name))
	_, _ = w.Write(artifact.Payload)
}

func (h *Handler) handleDeviceRegister(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterDeviceRequest
	if err := decodeJSON(r, &req, h.maxJSONBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := h.svc.RegisterDevice(req)
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleDeviceHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req service.DeviceAuthRequest
	if err := decodeJSON(r, &req, h.maxJSONBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.svc.DeviceHeartbeat(req); err != nil {
		writeErrorFromDomain(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleDeviceTelemetry(w http.ResponseWriter, r *http.Request) {
	var req service.DeviceTelemetryRequest
	if err := decodeJSON(r, &req, h.maxJSONBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.svc.DeviceTelemetry(req); err != nil {
		writeErrorFromDomain(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleDeviceLocation(w http.ResponseWriter, r *http.Request) {
	var req service.DeviceLocationRequest
	if err := decodeJSON(r, &req, h.maxJSONBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.svc.DeviceLocation(req); err != nil {
		writeErrorFromDomain(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleDevicePullCommand(w http.ResponseWriter, r *http.Request) {
	var req service.DevicePullRequest
	if err := decodeJSON(r, &req, h.maxJSONBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	command, err := h.svc.DevicePullCommand(req)
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}

	if command == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"command": nil})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"command": command})
}

func (h *Handler) handleDeviceCommandResult(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("command_id")

	var req service.DeviceCommandResultRequest
	if err := decodeJSON(r, &req, h.maxJSONBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.CommandID = commandID

	command, err := h.svc.DeviceCommandResult(req)
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}

	writeJSON(w, http.StatusOK, command)
}

func (h *Handler) handleDeviceGetArtifact(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	deviceToken := strings.TrimSpace(r.URL.Query().Get("device_token"))
	artifactID := r.PathValue("artifact_id")

	artifact, err := h.svc.DeviceGetArtifact(deviceID, deviceToken, artifactID)
	if err != nil {
		writeErrorFromDomain(w, err)
		return
	}

	w.Header().Set("Content-Type", artifact.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifact.Name))
	_, _ = w.Write(artifact.Payload)
}

func (h *Handler) requireOperator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, errors.New("missing bearer token"))
			return
		}

		if err := h.svc.RequireOperator(token); err != nil {
			writeErrorFromDomain(w, err)
			return
		}

		next(w, r)
	}
}

func (h *Handler) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("%s %s\n", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) withRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		ip := requestIP(r, h.trustProxyHeaders)
		if !h.apiRateLimiter.allow(ip, time.Now().UTC()) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, errors.New("rate limit exceeded"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (h *Handler) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(self), microphone=(), camera=()")
		w.Header().Set(
			"Content-Security-Policy",
			"default-src 'self'; script-src 'self' https://unpkg.com; style-src 'self' 'unsafe-inline' https://unpkg.com https://fonts.googleapis.com; "+
				"img-src 'self' data: https://*.tile.openstreetmap.org; font-src 'self' data: https://fonts.gstatic.com; "+
				"connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'",
		)

		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(r *http.Request, out interface{}, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 64 * 1024
	}

	defer r.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if int64(len(payload)) > maxBytes {
		return fmt.Errorf("request body too large")
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}

	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("invalid json: trailing content")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]interface{}{
		"error": err.Error(),
	})
}

func writeErrorFromDomain(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidPassword):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, auth.ErrInvalidToken):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, store.ErrFleetLimitReached):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, store.ErrDeviceExistsWithOtherIdentity):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, store.ErrDeviceNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, store.ErrInvalidDeviceToken):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, store.ErrCommandNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, store.ErrArtifactNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "required") || strings.Contains(message, "unsupported") || strings.Contains(message, "invalid") {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
	}
}

func bearerToken(header string) string {
	prefix := "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func parseIntOrDefault(raw string, def int) int {
	if raw == "" {
		return def
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return def
	}
	return value
}

func operatorFromRequest(_ *http.Request) string {
	return "operator"
}
