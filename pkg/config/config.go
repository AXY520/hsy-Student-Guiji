package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port        int    `yaml:"port"`
		UploadDir   string `yaml:"upload_dir"`
		MaxFileSize int64  `yaml:"max_file_size"`
	} `yaml:"server"`

	Map struct {
		APIKey        string `yaml:"api_key"`
		DefaultCenter struct {
			Latitude  float64 `yaml:"latitude"`
			Longitude float64 `yaml:"longitude"`
		} `yaml:"default_center"`
		DefaultZoom int `yaml:"default_zoom"`
	} `yaml:"map"`

	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`

	Logging struct {
		Level string `yaml:"level"`
		File  string `yaml:"file"`
	} `yaml:"logging"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
