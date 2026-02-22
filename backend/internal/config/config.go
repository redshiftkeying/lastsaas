package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Environment string           `yaml:"-"`
	Server      ServerConfig     `yaml:"server"`
	Database    DatabaseConfig   `yaml:"database"`
	Frontend    FrontendConfig   `yaml:"frontend"`
	JWT         JWTConfig        `yaml:"jwt"`
	OAuth       OAuthConfig      `yaml:"oauth"`
	Email       EmailConfig      `yaml:"email"`
	App         AppConfig        `yaml:"app"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type DatabaseConfig struct {
	URI  string `yaml:"uri"`
	Name string `yaml:"name"`
}

type FrontendConfig struct {
	URL       string `yaml:"url"`
	StaticDir string `yaml:"static_dir"`
}

type JWTConfig struct {
	AccessSecret  string `yaml:"access_secret"`
	RefreshSecret string `yaml:"refresh_secret"`
	AccessTTLMin  int    `yaml:"access_ttl_minutes"`
	RefreshTTLDay int    `yaml:"refresh_ttl_days"`
}

type OAuthConfig struct {
	GoogleClientID     string `yaml:"google_client_id"`
	GoogleClientSecret string `yaml:"google_client_secret"`
	GoogleRedirectURL  string `yaml:"google_redirect_url"`
}

type EmailConfig struct {
	ResendAPIKey string `yaml:"resend_api_key"`
	FromEmail    string `yaml:"from_email"`
	FromName     string `yaml:"from_name"`
}

type AppConfig struct {
	Name string `yaml:"name"`
}

// LoadEnvFile loads a .env file into the process environment if it exists.
// It searches the current directory and up to 3 parent directories.
func LoadEnvFile() {
	dir, _ := os.Getwd()
	for i := 0; i < 4; i++ {
		envPath := filepath.Join(dir, ".env")
		data, err := os.ReadFile(envPath)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if idx := strings.IndexByte(line, '='); idx > 0 {
					key := strings.TrimSpace(line[:idx])
					val := strings.TrimSpace(line[idx+1:])
					// Don't overwrite existing env vars
					if os.Getenv(key) == "" {
						os.Setenv(key, val)
					}
				}
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}

func Load(env string) (*Config, error) {
	configDir := os.Getenv("LASTSAAS_CONFIG_DIR")
	if configDir == "" {
		configDir = "config"
	}

	filename := fmt.Sprintf("%s.yaml", env)
	configPath := filepath.Join(configDir, filename)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	configStr := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(configStr), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg.Environment = env

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	var missing []string
	if c.Database.URI == "" {
		missing = append(missing, "database.uri")
	}
	if c.Database.Name == "" {
		missing = append(missing, "database.name")
	}
	if c.JWT.AccessSecret == "" {
		missing = append(missing, "jwt.access_secret")
	}
	if c.JWT.RefreshSecret == "" {
		missing = append(missing, "jwt.refresh_secret")
	}
	if c.Server.Port == 0 {
		missing = append(missing, "server.port")
	}
	if c.Frontend.URL == "" {
		missing = append(missing, "frontend.url")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func expandEnvVars(s string) string {
	return os.Expand(s, func(key string) string {
		// Support ${VAR:default} syntax
		if idx := strings.Index(key, ":"); idx >= 0 {
			envKey := key[:idx]
			defaultVal := key[idx+1:]
			if val := os.Getenv(envKey); val != "" {
				return val
			}
			return defaultVal
		}
		return os.Getenv(key)
	})
}

func GetEnv() string {
	env := os.Getenv("LASTSAAS_ENV")
	if env == "" {
		return "dev"
	}
	return env
}
