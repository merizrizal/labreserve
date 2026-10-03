package database

import "time"

type Role string

const (
	RoleEngineer    Role = "engineer"
	RoleCoordinator Role = "coordinator"
)

type Account struct {
	ID           string
	Login        string
	DisplayName  string
	PasswordHash string
	Role         Role
}

type Identity struct {
	ID          string
	Login       string
	DisplayName string
	Role        Role
}

type Resource struct {
	ID          string
	Code        string
	Name        string
	Description string
	Active      bool
}

type Session struct {
	TokenHash []byte
	AccountID *string
	CSRFToken []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	Identity  *Identity
}
