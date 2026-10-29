package platform

import "testing"

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
