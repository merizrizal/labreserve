package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DemoAccount struct {
	Login       string
	DisplayName string
	Role        Role
}

type seedResource struct {
	Code        string
	Name        string
	Description string
}

var demoAccounts = []DemoAccount{
	{Login: "alex@example.test", DisplayName: "Alex Morgan", Role: RoleEngineer},
	{Login: "sam@example.test", DisplayName: "Sam Rivera", Role: RoleEngineer},
	{Login: "jordan@example.test", DisplayName: "Jordan Lee", Role: RoleCoordinator},
}

var demoResources = []seedResource{
	{Code: "NET-01", Name: "Network Test Bench", Description: "Shared environment for network integration tests."},
	{Code: "K8S-01", Name: "Kubernetes Integration Lab", Description: "Shared cluster for application integration testing."},
	{Code: "DEMO-01", Name: "Customer Demo Environment", Description: "Shared environment for demonstrations."},
}

func DemoAccounts() []DemoAccount {
	return append([]DemoAccount(nil), demoAccounts...)
}

// Seed inserts the approved fictional accounts and resources without updating existing rows.
func Seed(ctx context.Context, pool *pgxpool.Pool, passwordHashes map[string]string) error {
	for _, account := range demoAccounts {
		if passwordHashes[account.Login] == "" {
			return fmt.Errorf("missing password hash for demo account %s", account.Login)
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, account := range demoAccounts {
		_, err := tx.Exec(ctx, `
			INSERT INTO accounts (id, login, display_name, password_hash, role)
			VALUES (gen_random_uuid(), $1, $2, $3, $4)
			ON CONFLICT (login) DO NOTHING`, account.Login, account.DisplayName, passwordHashes[account.Login], account.Role)
		if err != nil {
			return fmt.Errorf("seed demo account %s", account.Login)
		}
	}
	for _, resource := range demoResources {
		_, err := tx.Exec(ctx, `
			INSERT INTO resources (id, code, name, description, active)
			VALUES (gen_random_uuid(), $1, $2, $3, TRUE)
			ON CONFLICT (lower(code)) DO NOTHING`, resource.Code, resource.Name, resource.Description)
		if err != nil {
			return fmt.Errorf("seed demo resource %s", resource.Code)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit demo seed: %w", err)
	}
	return nil
}
