package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"lte_swd/backend/server/internal/model"
)

func TestRegisterFleetLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	st, err := NewStateStore(filepath.Join(dir, "state.json"), 1)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	now := time.Unix(100, 0).UTC()
	_, _, err = st.RegisterDevice("dev-1", "uid-1", "imei-1", "iccid-1", "r1", now)
	if err != nil {
		t.Fatalf("register first: %v", err)
	}

	_, _, err = st.RegisterDevice("dev-2", "uid-2", "imei-2", "iccid-2", "r1", now)
	if err == nil {
		t.Fatalf("expected fleet limit error")
	}
	if err != ErrFleetLimitReached {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommandLifecycle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	st, err := NewStateStore(filepath.Join(dir, "state.json"), 10)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	now := time.Unix(200, 0).UTC()
	device, _, err := st.RegisterDevice("dev-1", "uid-1", "imei-1", "iccid-1", "r1", now)
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	cmd, err := st.AddCommand("dev-1", "swd_reset", []byte(`{"hard":true}`), "operator", now)
	if err != nil {
		t.Fatalf("add command: %v", err)
	}

	pulled, err := st.PullNextCommand("dev-1", device.DeviceToken, now.Add(time.Second))
	if err != nil {
		t.Fatalf("pull command: %v", err)
	}
	if pulled == nil || pulled.CommandID != cmd.CommandID {
		t.Fatalf("unexpected command pulled: %#v", pulled)
	}
	if pulled.Status != model.CommandDispatched {
		t.Fatalf("expected dispatched status, got %s", pulled.Status)
	}

	done, err := st.CompleteCommand("dev-1", device.DeviceToken, cmd.CommandID, model.CommandResult{
		Status:  model.CommandSuccess,
		Message: "ok",
	}, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("complete command: %v", err)
	}
	if done.Status != model.CommandSuccess {
		t.Fatalf("expected success status, got %s", done.Status)
	}
}

func TestStorePersistence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	first, err := NewStateStore(stateFile, 10)
	if err != nil {
		t.Fatalf("new store first: %v", err)
	}

	now := time.Unix(300, 0).UTC()
	_, _, err = first.RegisterDevice("dev-1", "uid-1", "imei-1", "iccid-1", "r1", now)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not written: %v", err)
	}

	second, err := NewStateStore(stateFile, 10)
	if err != nil {
		t.Fatalf("new store second: %v", err)
	}

	if second.DeviceCount() != 1 {
		t.Fatalf("expected one device after reload")
	}
}
