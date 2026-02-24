// Package config handles jeltz configuration loading and validation.
package config

// Config holds the fully resolved jeltz configuration.
type Config struct {
	Listen           string
	ConfigFile       string
	BasePath         string
	DataDir          string
	LogLevel         string
	InsecureUpstream bool
	DumpTraffic      bool
	MaxBodyBytes     int64
}
