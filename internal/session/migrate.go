package session

import (
	"fmt"

	"github.com/adam/tau/internal/types"
)

// SupportedVersions lists all session header versions this package can read.
var SupportedVersions = []int{1}

// ValidateVersion checks the session header version against supported versions.
// Returns nil if the version is supported, error otherwise.
func ValidateVersion(version int) error {
	for _, v := range SupportedVersions {
		if version == v {
			return nil
		}
	}
	return fmt.Errorf("unsupported session version: %d (supported: %v)", version, SupportedVersions)
}

// migrateV1ToV2 is a stub for future version migration.
// Currently v1 is the only version, so this returns entries unchanged.
// When v2 is introduced, this function will transform v1 entries to v2 format.
func migrateV1ToV2(header *types.SessionHeader, entries []types.SessionEntry) (*types.SessionHeader, []types.SessionEntry, error) {
	// TODO: implement when v2 session format is defined
	return header, entries, nil
}
