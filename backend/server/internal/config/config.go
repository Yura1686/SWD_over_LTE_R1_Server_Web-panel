package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config keeps runtime settings for backend process.
type Config struct {
	HTTPAddr           string
	HTTPSAddr          string
	TLSCertFile        string
	TLSKeyFile         string
	OperatorPassword   string
	DeviceEnrollKey    string
	DataFile           string
	StaticDir          string
	FleetLimit         int
	OperatorTokenTTL   time.Duration
	DeviceOfflineAfter time.Duration
	MaxJSONBytes       int64
	MaxArtifactBytes   int64
	APIRatePerMinute   int
	LoginRatePerMinute int
	LoginBurst         int
	TrustProxyHeaders  bool
}

// Load reads environment variables and applies defaults for R1.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:           getEnv("HTTP_ADDR", ":8080"),
		HTTPSAddr:          getEnv("HTTPS_ADDR", ""),
		TLSCertFile:        getEnv("TLS_CERT_FILE", ""),
		TLSKeyFile:         getEnv("TLS_KEY_FILE", ""),
		OperatorPassword:   strings.TrimSpace(getEnv("OPERATOR_PASSWORD", "lte_swd_admin")),
		DeviceEnrollKey:    strings.TrimSpace(getEnv("DEVICE_ENROLL_KEY", "r1-enroll-key")),
		DataFile:           getEnv("DATA_FILE", "data/state.json"),
		StaticDir:          getEnv("STATIC_DIR", "../../web/panel"),
		FleetLimit:         getEnvInt("FLEET_LIMIT", 10),
		OperatorTokenTTL:   getEnvDuration("OPERATOR_TOKEN_TTL", 12*time.Hour),
		DeviceOfflineAfter: getEnvDuration("DEVICE_OFFLINE_AFTER", 90*time.Second),
		MaxJSONBytes:       int64(getEnvInt("MAX_JSON_BYTES", 64*1024)),
		MaxArtifactBytes:   int64(getEnvInt("MAX_ARTIFACT_BYTES", 12*1024*1024)),
		APIRatePerMinute:   getEnvInt("API_RATE_PER_MINUTE", 180),
		LoginRatePerMinute: getEnvInt("LOGIN_RATE_PER_MINUTE", 20),
		LoginBurst:         getEnvInt("LOGIN_BURST", 5),
		TrustProxyHeaders:  getEnvBool("TRUST_PROXY_HEADERS", false),
	}

	if cfg.FleetLimit <= 0 {
		return Config{}, fmt.Errorf("fleet limit must be positive")
	}
	if cfg.OperatorPassword == "" {
		return Config{}, fmt.Errorf("operator password must not be empty")
	}
	if cfg.DeviceEnrollKey == "" {
		return Config{}, fmt.Errorf("device enroll key must not be empty")
	}
	if cfg.MaxJSONBytes < 1024 {
		return Config{}, fmt.Errorf("max json bytes too small")
	}
	if cfg.MaxArtifactBytes < cfg.MaxJSONBytes {
		return Config{}, fmt.Errorf("max artifact bytes must be >= max json bytes")
	}
	if cfg.APIRatePerMinute <= 0 || cfg.LoginRatePerMinute <= 0 || cfg.LoginBurst <= 0 {
		return Config{}, fmt.Errorf("rate limits must be positive")
	}
	if (cfg.HTTPSAddr != "" || cfg.TLSCertFile != "" || cfg.TLSKeyFile != "") &&
		(cfg.HTTPSAddr == "" || cfg.TLSCertFile == "" || cfg.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("https requires HTTPS_ADDR, TLS_CERT_FILE and TLS_KEY_FILE together")
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return parsed
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return parsed
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return def
	}
}
