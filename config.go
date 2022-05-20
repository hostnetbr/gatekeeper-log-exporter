package main

import (
	"fmt"
	"os"

	"github.com/hostnetbr/gatekeeper-log-exporter/exporter/influx"
	yaml "gopkg.in/yaml.v2"
)

type Config struct {
	GkLogDir string         `yaml:"gk_log_dir"`
	InfluxDB *influx.Config `yaml:"influxdb"`
}

func parseConfig() (Config, error) {
	data, err := os.ReadFile(confFile)
	if err != nil {
		return Config{}, fmt.Errorf("error reading config file: %w", err)
	}

	var cfg Config
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("error parsing config: %w", err)
	}

	if cfg.GkLogDir == "" {
		return Config{}, fmt.Errorf("gk_log_dir empty: %w", err)
	}

	if cfg.InfluxDB == nil {
		return Config{}, fmt.Errorf("error parsing influxdb config")
	}
	if cfg.InfluxDB.User == "" || cfg.InfluxDB.Pass == "" || cfg.InfluxDB.Database == "" {
		return Config{}, fmt.Errorf("not enough authentication credentials for influxdb")
	}
	if cfg.InfluxDB.Hostname == "" {
		if cfg.InfluxDB.Hostname, err = os.Hostname(); err != nil {
			return Config{}, fmt.Errorf("error parsing hostname: %w", err)
		}
	}

	return cfg, nil
}
