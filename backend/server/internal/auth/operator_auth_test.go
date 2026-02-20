package auth

import (
	"testing"
	"time"
)

func TestOperatorAuthLifecycle(t *testing.T) {
	t.Parallel()

	a := NewOperatorAuth("secret", time.Hour)
	now := time.Unix(1000, 0).UTC()

	token, _, err := a.Login("secret", now)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if err := a.Validate(token, now.Add(10*time.Minute)); err != nil {
		t.Fatalf("validate failed: %v", err)
	}

	if err := a.Validate(token, now.Add(2*time.Hour)); err == nil {
		t.Fatalf("expected expired token")
	}
}
