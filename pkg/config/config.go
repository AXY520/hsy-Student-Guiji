package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port         int    `yaml:"port"`
		UploadDir    string `yaml:"upload_dir"`
		MaxFileSize  int64  `yaml:"max_file_size"`
		Host         string `yaml:"host"`
		ReadTimeout  int    `yaml:"read_timeout"`
		WriteTimeout int    `yaml:"write_timeout"`
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
		Path            string `yaml:"path"`
		MaxOpenConns    int    `yaml:"max_open_conns"`
		MaxIdleConns    int    `yaml:"max_idle_conns"`
		ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
	} `yaml:"database"`

	Logging struct {
		Level string `yaml:"level"`
		File  string `yaml:"file"`
	} `yaml:"logging"`

	Security struct {
		EnableHTTPS    bool     `yaml:"enable_https"`
		CertFile       string   `yaml:"cert_file"`
		KeyFile        string   `yaml:"key_file"`
		AllowedOrigins []string `yaml:"allowed_origins"`
		IPWhitelist    []string `yaml:"ip_whitelist"`
	} `yaml:"security"`

	RateLimit struct {
		RequestsPerSecond float64 `yaml:"requests_per_second"`
		Burst             int     `yaml:"burst"`
	} `yaml:"rate_limit"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	// 替换环境变量
	content := string(data)
	content = expandEnvVars(content)

	var config Config
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil, err
	}

	// 设置默认值
	setDefaults(&config)

	// 验证配置
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// expandEnvVars 展开环境变量
func expandEnvVars(content string) string {
	return os.Expand(content, func(key string) string {
		return os.Getenv(key)
	})
}

// setDefaults 设置默认值
func setDefaults(config *Config) {
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.Server.ReadTimeout == 0 {
		config.Server.ReadTimeout = 30
	}
	if config.Server.WriteTimeout == 0 {
		config.Server.WriteTimeout = 30
	}
	if config.Database.MaxOpenConns == 0 {
		config.Database.MaxOpenConns = 10
	}
	if config.Database.MaxIdleConns == 0 {
		config.Database.MaxIdleConns = 5
	}
	if config.Database.ConnMaxLifetime == 0 {
		config.Database.ConnMaxLifetime = 3600 // 1 hour
	}
	if config.RateLimit.RequestsPerSecond == 0 {
		config.RateLimit.RequestsPerSecond = 10
	}
	if config.RateLimit.Burst == 0 {
		config.RateLimit.Burst = 20
	}
}

// validateConfig 验证配置
func validateConfig(config *Config) error {
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("无效的端口号: %d", config.Server.Port)
	}
	if config.Server.UploadDir == "" {
		return fmt.Errorf("上传目录不能为空")
	}
	if config.Map.APIKey == "" {
		return fmt.Errorf("地图API密钥不能为空")
	}
	if config.Database.Path == "" {
		return fmt.Errorf("数据库路径不能为空")
	}
	return nil
}
