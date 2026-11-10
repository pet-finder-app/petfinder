package auth

import (
	"errors"
	"fmt"
)

type Role string

const (
	RoleAdopter    Role = "adopter"
	RoleAdvertiser Role = "advertiser"
	RoleNGOMember  Role = "organization_member"
	RoleAdmin      Role = "platform_admin"
)

var ErrInvalidRole = errors.New("invalid role")

func (r Role) Valid() bool {
	switch r {
	case RoleAdopter, RoleAdvertiser, RoleNGOMember, RoleAdmin:
		return true
	default:
		return false
	}
}

func validateRoles(roles []Role) error {
	for _, role := range roles {
		if !role.Valid() {
			return fmt.Errorf("%w: %q", ErrInvalidRole, role)
		}
	}
	return nil
}

func uniqueRoles(roles []Role) []Role {
	unique := make([]Role, 0, len(roles))
	seen := make(map[Role]struct{}, len(roles))
	for _, role := range roles {
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		unique = append(unique, role)
	}
	return unique
}
