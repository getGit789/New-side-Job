package config

import "testing"

func TestLoadDefaultsAndValidation(t *testing.T) {
	t.Setenv("BRIEFRELAY_ENV", "development")
	t.Setenv("BRIEFRELAY_DATA_DIR", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL.String() != "http://localhost:8080" || c.MaxUploadMB != 50 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	t.Setenv("BRIEFRELAY_ENV", "production")
	t.Setenv("BRIEFRELAY_BASE_URL", "http://insecure.example")
	if _, err := Load(); err == nil {
		t.Fatal("production with http base URL must fail")
	}
}
