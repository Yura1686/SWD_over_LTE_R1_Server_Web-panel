package store

import "errors"

var (
	// ErrFleetLimitReached indicates that registration exceeds R1 fleet cap.
	ErrFleetLimitReached = errors.New("fleet limit reached")
	// ErrDeviceExistsWithOtherIdentity indicates immutable fields changed unexpectedly.
	ErrDeviceExistsWithOtherIdentity = errors.New("device id already exists with different identity")
	// ErrDeviceNotFound indicates unknown device id.
	ErrDeviceNotFound = errors.New("device not found")
	// ErrInvalidDeviceToken indicates token mismatch.
	ErrInvalidDeviceToken = errors.New("invalid device token")
	// ErrCommandNotFound indicates unknown command id for a device.
	ErrCommandNotFound = errors.New("command not found")
	// ErrArtifactNotFound indicates unknown artifact id.
	ErrArtifactNotFound = errors.New("artifact not found")
)
