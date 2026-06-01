// Copyright 2026 Mathias Schwenk
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rrpgwd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Host != "xrrp-ote.rrpproxy.net" || cfg.ListenPort != "2000" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.AuthUser != "test" || cfg.AuthPassword != "test" {
		t.Errorf("auth defaults changed: %q/%q", cfg.AuthUser, cfg.AuthPassword)
	}
}

func TestLoadConfigYAMLAndEnvPrecedence(t *testing.T) {
	clearEnv(t)

	path := filepath.Join(t.TempDir(), "rrpgwd.yaml")
	yaml := "host: yaml-host\nusername: yaml-user\npool_min: 5\nauth_password: yamlpw\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	// env must win over YAML, YAML over defaults
	t.Setenv("RRPGWD_USERNAME", "env-user")
	t.Setenv("RRPGWD_POOL_MAX", "9")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Host != "yaml-host" {
		t.Errorf("YAML override failed: host=%q", cfg.Host)
	}
	if cfg.Username != "env-user" {
		t.Errorf("env should win over YAML: username=%q", cfg.Username)
	}
	if cfg.PoolMin != 5 {
		t.Errorf("YAML pool_min failed: %d", cfg.PoolMin)
	}
	if cfg.PoolMax != 9 {
		t.Errorf("env pool_max failed: %d", cfg.PoolMax)
	}
	if cfg.AuthPassword != "yamlpw" {
		t.Errorf("YAML auth_password failed: %q", cfg.AuthPassword)
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"RRPGWD_CONFIG", "RRPGWD_HOST", "RRPGWD_PORT", "RRPGWD_USERNAME", "RRPGWD_PASSWORD",
		"RRPGWD_LISTEN_HOST", "RRPGWD_LISTEN_PORT", "RRPGWD_POOL_MIN", "RRPGWD_POOL_MAX",
		"RRPGWD_AUTH_USER", "RRPGWD_AUTH_PASSWORD",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}
