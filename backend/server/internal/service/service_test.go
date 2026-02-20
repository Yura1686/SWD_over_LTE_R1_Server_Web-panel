package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"lte_swd/backend/server/internal/auth"
	"lte_swd/backend/server/internal/config"
	"lte_swd/backend/server/internal/store"
)

func TestOperatorCreateCommandValidation(t *testing.T) {
	t.Parallel()

	st, err := store.NewStateStore(filepath.Join(t.TempDir(), "state.json"), 10)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	cfg := config.Config{
		DeviceEnrollKey:    "enroll",
		DeviceOfflineAfter: 30 * time.Second,
	}

	svc := New(cfg, st, auth.NewOperatorAuth("pass", time.Hour))

	_, err = svc.RegisterDevice(RegisterDeviceRequest{
		EnrollKey:       "enroll",
		DeviceID:        "dev-1",
		HWUID:           "uid",
		ModemIMEI:       "imei",
		SimICCID:        "iccid",
		FirmwareVersion: "r1",
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	_, err = svc.OperatorCreateCommand(OperatorCommandRequest{
		DeviceID: "dev-1",
		Type:     "unsupported",
		Payload:  json.RawMessage(`{}`),
	}, "operator")
	if err == nil {
		t.Fatalf("expected error for unsupported command")
	}
}
