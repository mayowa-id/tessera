package platform

import "testing"

func TestPrefixedDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("TS_POSTGRES_URL", "postgres://prefixed/database?sslmode=require")
	t.Setenv("TS_DATABASE_URL", "postgres://fallback/database?sslmode=require")
	t.Setenv("APP_ORIGIN", "https://tessera-ui.vercel.app")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("PORT", "")
	c, err := LoadConfig()
	if err != nil || c.DatabaseURL != "postgres://prefixed/database?sslmode=require" {
		t.Fatalf("prefixed configuration: %+v %v", c, err)
	}
	t.Setenv("DATABASE_URL", "postgres://explicit/database?sslmode=require")
	c, err = LoadConfig()
	if err != nil || c.DatabaseURL != "postgres://explicit/database?sslmode=require" {
		t.Fatalf("explicit precedence: %+v %v", c, err)
	}
}

func TestListenAddress(t *testing.T) {
	for _, tc := range []struct {
		name, port, address, want string
		invalid                   bool
	}{
		{"local default", "", "", ":8080", false},
		{"local override", "", "127.0.0.1:8081", "127.0.0.1:8081", false},
		{"Vercel port takes precedence", "3000", ":8081", ":3000", false},
		{"invalid deployment port", "invalid", "", "", true},
		{"out of range deployment port", "65536", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VERCEL", "")
			t.Setenv("APP_ORIGIN", "http://localhost:4200")
			t.Setenv("SESSION_COOKIE_SECURE", "false")
			t.Setenv("PORT", tc.port)
			t.Setenv("HTTP_ADDR", tc.address)
			t.Setenv("DATABASE_URL", "postgres://localhost/tessera")
			cfg, err := LoadConfig()
			if tc.invalid {
				if err == nil {
					t.Fatal("expected invalid PORT to fail")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Address != tc.want {
				t.Fatalf("address=%q, want %q", cfg.Address, tc.want)
			}
		})
	}
}

func TestBrowserSecurityConfig(t *testing.T) {
	for _, tc := range []struct {
		name, origin, secure, vercel string
		invalid                      bool
	}{
		{"local development", "http://localhost:4200", "false", "", false},
		{"production", "https://tessera.example.com", "true", "1", false},
		{"production origin required", "", "true", "1", true},
		{"production secure cookies required", "https://tessera.example.com", "false", "1", true},
		{"production HTTPS required", "http://tessera.example.com", "true", "1", true},
		{"origin path rejected", "https://tessera.example.com/login", "true", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/tessera")
			t.Setenv("HTTP_ADDR", ":8080")
			t.Setenv("PORT", "")
			t.Setenv("APP_ORIGIN", tc.origin)
			t.Setenv("SESSION_COOKIE_SECURE", tc.secure)
			t.Setenv("VERCEL", tc.vercel)
			_, err := LoadConfig()
			if (err != nil) != tc.invalid {
				t.Fatalf("invalid=%v, error=%v", tc.invalid, err)
			}
		})
	}
}
