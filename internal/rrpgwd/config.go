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

// Package rrpgwd implements the rrpgwd daemon: a local TCP front end that accepts
// PTF requests, holds a pool of authenticated TLS sessions to an upstream XRRP
// server, forwards each request, and returns the response. It is consumed only
// by cmd/; code under pkg/ must not import it.
package rrpgwd

import (
	"crypto/tls"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds the daemon's full configuration: upstream connection and
// credentials, the local listener, session-pool sizing, local-side client
// authentication, and the upstream TLS settings. It is passed around as a
// pointer because TLSConfig contains a mutex and must not be copied.
type Config struct {
	// Upstream XRRP server
	Host     string
	Port     string
	Username string
	Password string

	// Local listener
	ListenHost string
	ListenPort string

	// Session pool sizing
	PoolMin int
	PoolMax int

	// Session keepalive: the janitor pings idle sessions every
	// KeepaliveSeconds (keep below the upstream idle timeout) and retires
	// sessions idle beyond IdleTimeoutSeconds while above PoolMin.
	KeepaliveSeconds   int
	IdleTimeoutSeconds int

	// Local-side authentication for connecting clients
	AuthUser     string
	AuthPassword string

	TLSConfig tls.Config
}

// yamlConfig mirrors Config for YAML decoding (TLSConfig is excluded).
type yamlConfig struct {
	Host               string `yaml:"host"`
	Port               string `yaml:"port"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	ListenHost         string `yaml:"listen_host"`
	ListenPort         string `yaml:"listen_port"`
	PoolMin            *int   `yaml:"pool_min"`
	PoolMax            *int   `yaml:"pool_max"`
	KeepaliveSeconds   *int   `yaml:"keepalive_seconds"`
	IdleTimeoutSeconds *int   `yaml:"idle_timeout_seconds"`
	AuthUser           string `yaml:"auth_user"`
	AuthPassword       string `yaml:"auth_password"`
}

// NewConfig returns the built-in defaults. No secrets are baked in.
func NewConfig() Config {
	return Config{
		Host:               "xrrp-ote.rrpproxy.net",
		Port:               "2001",
		ListenHost:         "",
		ListenPort:         "2000",
		PoolMin:            3,
		PoolMax:            3,
		KeepaliveSeconds:   540,
		IdleTimeoutSeconds: 600,
		AuthUser:           "test",
		AuthPassword:       "test",
		TLSConfig:          tls.Config{},
	}
}

// LoadConfig builds a Config from (in increasing precedence): built-in
// defaults, an optional YAML file, and environment variables. An empty path
// falls back to $RRPGWD_CONFIG, then "./rrpgwd.yaml". A missing file is not an
// error.
func LoadConfig(path string) (*Config, error) {
	cfg := NewConfig()

	if path == "" {
		path = os.Getenv("RRPGWD_CONFIG")
	}
	if path == "" {
		path = "rrpgwd.yaml"
	}

	if data, err := os.ReadFile(path); err == nil {
		var yc yamlConfig
		if err := yaml.Unmarshal(data, &yc); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
		applyYAML(&cfg, &yc)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	applyEnv(&cfg)

	return &cfg, nil
}

// applyYAML overlays non-empty values from a decoded YAML file onto cfg.
func applyYAML(cfg *Config, yc *yamlConfig) {
	setString(&cfg.Host, yc.Host)
	setString(&cfg.Port, yc.Port)
	setString(&cfg.Username, yc.Username)
	setString(&cfg.Password, yc.Password)
	setString(&cfg.ListenHost, yc.ListenHost)
	setString(&cfg.ListenPort, yc.ListenPort)
	setString(&cfg.AuthUser, yc.AuthUser)
	setString(&cfg.AuthPassword, yc.AuthPassword)
	if yc.PoolMin != nil {
		cfg.PoolMin = *yc.PoolMin
	}
	if yc.PoolMax != nil {
		cfg.PoolMax = *yc.PoolMax
	}
	if yc.KeepaliveSeconds != nil {
		cfg.KeepaliveSeconds = *yc.KeepaliveSeconds
	}
	if yc.IdleTimeoutSeconds != nil {
		cfg.IdleTimeoutSeconds = *yc.IdleTimeoutSeconds
	}
}

// applyEnv overlays RRPGWD_* environment variables onto cfg; these take
// precedence over both the defaults and the YAML file.
func applyEnv(cfg *Config) {
	setFromEnv(&cfg.Host, "RRPGWD_HOST")
	setFromEnv(&cfg.Port, "RRPGWD_PORT")
	setFromEnv(&cfg.Username, "RRPGWD_USERNAME")
	setFromEnv(&cfg.Password, "RRPGWD_PASSWORD")
	setFromEnv(&cfg.ListenHost, "RRPGWD_LISTEN_HOST")
	setFromEnv(&cfg.ListenPort, "RRPGWD_LISTEN_PORT")
	setFromEnv(&cfg.AuthUser, "RRPGWD_AUTH_USER")
	setFromEnv(&cfg.AuthPassword, "RRPGWD_AUTH_PASSWORD")
	setIntFromEnv(&cfg.PoolMin, "RRPGWD_POOL_MIN")
	setIntFromEnv(&cfg.PoolMax, "RRPGWD_POOL_MAX")
	setIntFromEnv(&cfg.KeepaliveSeconds, "RRPGWD_KEEPALIVE_SECONDS")
	setIntFromEnv(&cfg.IdleTimeoutSeconds, "RRPGWD_IDLE_TIMEOUT_SECONDS")
}

func setString(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

func setFromEnv(dst *string, key string) {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		*dst = v
	}
}

func setIntFromEnv(dst *int, key string) {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}
