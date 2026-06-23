package config

import (
	"github.com/Rake-Pro/goshareit/logger"
	"github.com/Rake-Pro/goshareit/security"
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	Uploader struct {
		Type string `yaml:"type"`
	} `yaml:"uploader"`

	Nextcloud struct {
		BaseURL         string `yaml:"base_url"`
		Username        string `yaml:"username"`
		Password        string `yaml:"password"`
		PublicURLPrefix string `yaml:"public_url_prefix"`
	} `yaml:"nextcloud"`

	Shortcuts struct {
		FullScreenshot      string `yaml:"full_screenshot"`
		SelectiveScreenshot string `yaml:"selective_screenshot"`
		StartStopRecording  string `yaml:"start_stop_recording"`
		Quit                string `yaml:"quit"`
	} `yaml:"shortcuts"`

	Notifications struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"notifications"`

	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`
}

var AppConfig Config

func LoadConfig() {
	configPath := os.Getenv("GOSHAREIT_CONFIG_PATH")
	if configPath == "" {
		configPath = "config/config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		logger.Fatal("Failed to read config file: " + err.Error())
	}

	if err := yaml.Unmarshal(data, &AppConfig); err != nil {
		logger.Fatal("Failed to parse YAML config: " + err.Error())
	}

	logger.SetLogLevel(AppConfig.Logging.Level)
	logger.Info("Configuration loaded from: " + configPath)

	redacted := security.RedactPasswords(AppConfig)

	dump, err := yaml.Marshal(&redacted)
	if err != nil {
		logger.Error("Failed to marshal redacted config: " + err.Error())
	} else {
		logger.Info("Loaded configuration (redacted):\n" + string(dump))
	}
}
