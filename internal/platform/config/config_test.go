package config

import "testing"

func TestConfigDefaultsAndValidation(t *testing.T) {
	for _, name := range []string{"APP_ADDR", "APP_DATA_SOURCE", "MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE"} {
		t.Setenv(name, "")
	}
	cfg, err := Load()
	if err != nil || cfg.DataSource != "demo" || cfg.Addr != "127.0.0.1:8080" || cfg.MySQL.Port != 3306 {
		t.Fatalf("unexpected defaults: %+v %v", cfg, err)
	}
	for _, tc := range []struct{ name, value string }{
		{"APP_DATA_SOURCE", "typo"}, {"APP_ADDR", "8080"}, {"MYSQL_PORT", "0"}, {"MYSQL_PORT", "65536"}, {"MYSQL_PORT", "abc"},
	} {
		t.Run(tc.name+tc.value, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
