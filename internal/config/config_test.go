package config

import "testing"

func TestDevelopmentCookieSecurityRequiresLocalBindingOrContainerOptIn(t *testing.T) {
	t.Run("loopback HTTP is local development", func(t *testing.T) {
		setConfigEnvironment(t, "development", "127.0.0.1:8080", "http://127.0.0.1:8080", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.CookieSecure {
			t.Fatal("loopback HTTP development should omit Secure for the local browser")
		}
	})

	t.Run("wildcard HTTP without container declaration is rejected", func(t *testing.T) {
		setConfigEnvironment(t, "development", "0.0.0.0:8080", "http://127.0.0.1:8080", "")
		if _, err := Load(); err == nil {
			t.Fatal("development HTTP accepted a wildcard bind without explicit container isolation")
		}
	})

	t.Run("containerized loopback-published HTTP is explicit", func(t *testing.T) {
		setConfigEnvironment(t, "development", "0.0.0.0:8080", "http://127.0.0.1:8080", "true")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.CookieSecure {
			t.Fatal("explicit loopback-published HTTP container should use a local non-Secure cookie")
		}
	})

	t.Run("HTTPS always uses Secure cookies", func(t *testing.T) {
		setConfigEnvironment(t, "development", "127.0.0.1:8080", "https://localhost", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.CookieSecure {
			t.Fatal("HTTPS development must use Secure cookies")
		}
	})
}

func setConfigEnvironment(t *testing.T, appEnv, listen, origin, containerized string) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost/labreserve")
	t.Setenv("APP_ENV", appEnv)
	t.Setenv("LISTEN_ADDR", listen)
	t.Setenv("APP_ORIGIN", origin)
	t.Setenv("APP_CONTAINERIZED", containerized)
}
