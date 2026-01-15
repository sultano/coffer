package config

// ProjectConfig represents the .coffer.yaml file
type ProjectConfig struct {
	Version      int                       `yaml:"version"`
	Config       ConfigSection             `yaml:"config"`
	GCP          GCPConfig                 `yaml:"gcp"`
	Environments map[string]EnvConfig      `yaml:"environments"`
	EnvMapping   map[string]string         `yaml:"env_mapping"`
	Defaults     DefaultsConfig            `yaml:"defaults"`
	Cache        CacheConfig               `yaml:"cache"`
}

type ConfigSection struct {
	Path string `yaml:"path"`
	Base string `yaml:"base"`
}

type GCPConfig struct {
	Project      string `yaml:"project"`
	SecretPrefix string `yaml:"secret_prefix"`
}

type EnvGCPConfig struct {
	Project string `yaml:"project"`
}

type EnvConfig struct {
	GCP EnvGCPConfig `yaml:"gcp"`
}

type DefaultsConfig struct {
	Env string `yaml:"env"`
}

type CacheConfig struct {
	Enabled bool   `yaml:"enabled"`
	TTL     string `yaml:"ttl"`
	Path    string `yaml:"path"`
}

// LoadedConfig holds the merged configuration with metadata
type LoadedConfig struct {
	Project     *ProjectConfig
	Environment string
	Values      map[string]any
	ProjectRoot string
}
