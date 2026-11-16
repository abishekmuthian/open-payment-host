package config

import (
	"strings"
	"testing"
)

// TestLoad tests load of broken json
func TestLoadBroken(t *testing.T) {

	c := New()

	// This should not load
	err := c.Load("testdata/bogus.json")
	if err == nil {
		t.Fatalf("config did not error on bad json")
	}

	// This should load
	err = c.Load("testdata/config.json")
	if err != nil {
		t.Fatalf("config failed to load valid json")
	}

	// This should not load as it does not have all configs
	// which could lead to issues with wrong config being used.
	err = c.Load("testdata/single.json")
	if err == nil {
		t.Fatalf("config did not error on single json")
	}

}

// TestConfig tests valid config.json
func TestConfig(t *testing.T) {

	// Load json
	c := New()
	err := c.Load("testdata/config.json")
	if err != nil {
		t.Fatalf("config failed to load valid json")
	}

	// Test we have some values we expect in dev mode
	if c.Get("assets_compiled") != "no" {
		t.Fatalf("config failed to load 'no' json")
	}

	if c.GetBool("assets_compiled") {
		t.Fatalf("config failed to load bool json")
	}

	if c.GetInt("assets_compiled") != 0 {
		t.Fatalf("config failed to reject non-int json")
	}

	if c.Get("mail_from") != "example@example.com" {
		t.Fatalf("config failed to load email from json")
	}

	if c.GetInt("port") != 3000 {
		t.Fatalf("config failed to load port int from json")
	}

	if c.Get("root_url") != "https://localhost:3000" {
		t.Fatalf("config failed to load email from json")
	}

	if c.Configuration(ModeTest)["root_url"] != "https://localhost:3000" {
		t.Fatalf("config failed to get all")
	}
}

// TestNilConfig checks that package helpers are safe before a config is loaded.
func TestNilConfig(t *testing.T) {
	old := Current
	Current = nil
	defer func() { Current = old }()
	if Production() || Get("x") != "" || GetInt("x") != 0 || GetBool("x") {
		t.Fatal("nil config returned values")
	}
	if _, err := GetBoolStrict("x"); err == nil {
		t.Fatal("strict getter did not report missing configuration")
	}
}

func TestGetBoolStrict(t *testing.T) {
	old := Current
	defer func() { Current = old }()
	for _, mode := range []int{ModeDevelopment, ModeProduction} {
		for _, value := range []string{"yes", "no", "", "true", "false", "1", "0", "on", "off", "YES", " yes ", "invalid-secret-value"} {
			c := New()
			c.Mode = mode
			c.configs[mode] = map[string]string{"flag": value}
			Current = c
			got, err := GetBoolStrict("flag")
			if value == "yes" || value == "no" {
				if err != nil || got != (value == "yes") || got != GetBool("flag") {
					t.Fatalf("mode=%d value=%q: got %v, %v", mode, value, got, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "flag") || got {
				t.Fatalf("mode=%d invalid value=%q: got %v, %v", mode, value, got, err)
			} else if value == "invalid-secret-value" && strings.Contains(err.Error(), value) {
				t.Fatal("configuration error exposed the supplied value")
			}
		}
	}
}
