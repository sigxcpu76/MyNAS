package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `mapstructure:"server" json:"server" yaml:"server"`
	System  SystemConfig  `mapstructure:"system" json:"system" yaml:"system"`
	Modules ModulesConfig `mapstructure:"modules" json:"modules" yaml:"modules"`
}

type ServerConfig struct {
	HTTPPort   int    `mapstructure:"http_port" json:"http_port" yaml:"http_port"`
	GRPCPort   int    `mapstructure:"grpc_port" json:"grpc_port" yaml:"grpc_port"`
	HybridMode bool   `mapstructure:"hybrid" json:"hybrid" yaml:"hybrid"`
	StateDir   string `mapstructure:"state_dir" json:"state_dir" yaml:"state_dir"`
	JWTSecret  string `mapstructure:"jwt_secret" json:"jwt_secret" yaml:"jwt_secret"`
}

type SystemConfig struct {
	MainPool      string `mapstructure:"main_pool" json:"main_pool" yaml:"main_pool"`
	SystemDataset string `mapstructure:"system_dataset" json:"system_dataset" yaml:"system_dataset"`
	AutoDiscover  bool   `mapstructure:"auto_discover" json:"auto_discover" yaml:"auto_discover"`
}

type ModulesConfig struct {
	ZFS    bool `mapstructure:"zfs" json:"zfs" yaml:"zfs"`
	KVM    bool `mapstructure:"kvm" json:"kvm" yaml:"kvm"`
	Docker bool `mapstructure:"docker" json:"docker" yaml:"docker"`
	LXC    bool `mapstructure:"lxc" json:"lxc" yaml:"lxc"`
	CIFS   bool `mapstructure:"cifs" json:"cifs" yaml:"cifs"`
	WebDAV bool `mapstructure:"webdav" json:"webdav" yaml:"webdav"`
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	stateDir := "/var/lib/nas"
	if os.Geteuid() != 0 {
		home, err := os.UserHomeDir()
		if err == nil {
			stateDir = filepath.Join(home, ".local", "share", "nas")
		} else {
			stateDir = "/tmp/nas-state"
		}
	}

	return &Config{
		Server: ServerConfig{
			HTTPPort:   8080,
			GRPCPort:   9090,
			HybridMode: false,
			StateDir:   stateDir,
			JWTSecret:  "nas-production-secret-2026",
		},
		System: SystemConfig{
			MainPool:      "",
			SystemDataset: ".nas-system",
			AutoDiscover:  true,
		},
		Modules: ModulesConfig{
			ZFS:    true,
			KVM:    true,
			Docker: true,
			LXC:    true,
			CIFS:   true,
			WebDAV: true,
		},
	}
}

var (
	globalMu  sync.RWMutex
	globalCfg *Config
	configPath string
)

// Load reads configuration from explicit path or default search paths using spf13/viper.
func Load(explicitPath string) (*Config, string, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix("NAS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// Set defaults in viper
	def := DefaultConfig()
	v.SetDefault("server.http_port", def.Server.HTTPPort)
	v.SetDefault("server.grpc_port", def.Server.GRPCPort)
	v.SetDefault("server.hybrid", def.Server.HybridMode)
	v.SetDefault("server.state_dir", def.Server.StateDir)
	v.SetDefault("server.jwt_secret", def.Server.JWTSecret)
	v.SetDefault("system.main_pool", def.System.MainPool)
	v.SetDefault("system.system_dataset", def.System.SystemDataset)
	v.SetDefault("system.auto_discover", def.System.AutoDiscover)
	v.SetDefault("modules.zfs", def.Modules.ZFS)
	v.SetDefault("modules.kvm", def.Modules.KVM)
	v.SetDefault("modules.docker", def.Modules.Docker)
	v.SetDefault("modules.lxc", def.Modules.LXC)
	v.SetDefault("modules.cifs", def.Modules.CIFS)
	v.SetDefault("modules.webdav", def.Modules.WebDAV)

	resolvedPath := ""

	if explicitPath != "" {
		v.SetConfigFile(explicitPath)
		resolvedPath = explicitPath
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("/etc/nas")
		if home, err := os.UserHomeDir(); err == nil {
			v.AddConfigPath(filepath.Join(home, ".config", "nas"))
		}
		v.AddConfigPath(".")
	}

	err := v.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok || os.IsNotExist(err) {
			// Create a default config file in candidate directory
			target := explicitPath
			if target == "" {
				if home, herr := os.UserHomeDir(); herr == nil && os.Geteuid() != 0 {
					target = filepath.Join(home, ".config", "nas", "config.yaml")
				} else if os.Geteuid() == 0 {
					target = "/etc/nas/config.yaml"
				} else {
					target = "config.yaml"
				}
			}
			if serr := Save(def, target); serr == nil {
				resolvedPath = target
			}
			globalMu.Lock()
			globalCfg = def
			configPath = resolvedPath
			globalMu.Unlock()
			return def, resolvedPath, nil
		}
		return nil, "", fmt.Errorf("failed to read config: %w", err)
	}

	resolvedPath = v.ConfigFileUsed()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, resolvedPath, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Always ensure ZFS module is true
	cfg.Modules.ZFS = true

	globalMu.Lock()
	globalCfg = &cfg
	configPath = resolvedPath
	globalMu.Unlock()

	return &cfg, resolvedPath, nil
}

// GetGlobal returns active configuration.
func GetGlobal() *Config {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalCfg == nil {
		return DefaultConfig()
	}
	cp := *globalCfg
	return &cp
}

// GetConfigPath returns path of currently loaded config file.
func GetConfigPath() string {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return configPath
}

// Save writes configuration to disk as YAML.
func Save(cfg *Config, targetPath string) error {
	dir := filepath.Dir(targetPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			// If cannot create directory, fallback to current directory
			targetPath = "config.yaml"
		}
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to yaml: %w", err)
	}

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config to %s: %w", targetPath, err)
	}

	globalMu.Lock()
	globalCfg = cfg
	configPath = targetPath
	globalMu.Unlock()

	return nil
}
