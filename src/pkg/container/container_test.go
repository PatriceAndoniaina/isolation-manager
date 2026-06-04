package container

import (
	"testing"

	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple", "user01", false},
		{"with hyphen", "user-01", false},
		{"min length", "ab", false},
		{"empty", "", true},
		{"too short", "a", true},
		{"leading digit", "1user", true},
		{"uppercase", "User01", true},
		{"underscore", "user_01", true},
		{"space", "user 01", true},
		{"slash injection", "user/../etc", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz0123456789", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateName(%q) = nil, want error", tt.input)
				}
				if !apperrors.Is(err, apperrors.ErrInvalidName) {
					t.Fatalf("ValidateName(%q): error %v, want ErrInvalidName", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateName(%q) = %v, want nil", tt.input, err)
			}
		})
	}
}

func TestDefaultLimits(t *testing.T) {
	l := DefaultLimits()
	if l.MemoryMB <= 0 || l.CPUQuota <= 0 || l.PidsLimit <= 0 {
		t.Fatalf("DefaultLimits() returned non-positive value: %+v", l)
	}
}
