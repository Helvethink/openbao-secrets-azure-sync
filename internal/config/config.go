package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level controller configuration.
type Config struct {
	OpenBao     OpenBaoConfig     `yaml:"openbao"`
	Watcher     WatcherConfig     `yaml:"watcher"`
	API         APIConfig         `yaml:"api"`
	Destination DestinationConfig `yaml:"destination"`
	State       StateConfig       `yaml:"state"`
	Azure       AzureConfig       `yaml:"azure"`
	Sync        SyncConfig        `yaml:"sync"`
}

// OpenBaoConfig configures the source OpenBao instance and mounted KV engines.
type OpenBaoConfig struct {
	InstanceName string   `yaml:"instance_name"`
	Namespace    string   `yaml:"namespace"`
	Namespaces   []string `yaml:"namespaces"`
	Address      string   `yaml:"address"`
	Token        string   `yaml:"token"`
	Mounts       []string `yaml:"mounts"`
}

// WatcherConfig selects the audit event source.
type WatcherConfig struct {
	Type         string            `yaml:"type"`
	AuditLogPath string            `yaml:"audit_log_path"`
	HTTP         HTTPWatcherConfig `yaml:"http"`
}

// HTTPWatcherConfig configures the HTTP audit-device endpoint.
type HTTPWatcherConfig struct {
	ListenAddress string `yaml:"listen_address"`
	Path          string `yaml:"path"`
	AuthToken     string `yaml:"auth_token"`
}

// APIConfig configures the operational API and metrics endpoint.
type APIConfig struct {
	Enabled       bool   `yaml:"enabled"`
	ListenAddress string `yaml:"listen_address"`
	AuthToken     string `yaml:"auth_token"`
}

// DestinationConfig selects the destination mode.
type DestinationConfig struct {
	Type string `yaml:"type"`
}

// StateConfig reserves configuration for future local state storage.
type StateConfig struct {
	Type string `yaml:"type"`
	DSN  string `yaml:"dsn"`
}

// AzureConfig configures Azure credentials and Key Vault overrides.
type AzureConfig struct {
	Auth           AzureAuthConfig            `yaml:"auth"`
	ResourceGroups []AzureResourceGroupConfig `yaml:"resource_groups"`
	KeyVaults      []AzureKeyVaultConfig      `yaml:"key_vaults"`
}

// AzureAuthConfig describes one Azure credential source.
type AzureAuthConfig struct {
	Type            string `yaml:"type"`
	TenantID        string `yaml:"tenant_id"`
	ClientID        string `yaml:"client_id"`
	ClientSecret    string `yaml:"client_secret"`
	ClientSecretEnv string `yaml:"client_secret_env"`
}

// AzureResourceGroupConfig configures resource-group-scoped Azure auth.
type AzureResourceGroupConfig struct {
	Name           string          `yaml:"name"`
	SubscriptionID string          `yaml:"subscription_id"`
	Auth           AzureAuthConfig `yaml:"auth"`
}

// AzureKeyVaultConfig configures Key Vault URL and auth overrides.
type AzureKeyVaultConfig struct {
	Name          string          `yaml:"name"`
	URL           string          `yaml:"url"`
	ResourceGroup string          `yaml:"resource_group"`
	Auth          AzureAuthConfig `yaml:"auth"`
}

// SyncConfig contains controller-level sync knobs.
type SyncConfig struct {
	DryRun      bool `yaml:"dry_run"`
	WorkerCount int  `yaml:"worker_count"`
}

// Load reads, normalizes, and validates a YAML configuration file.
func Load(path string) (Config, error) {
	cfg := defaultConfig()

	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return Config{}, err
	}

	normalizeDefaults(&cfg)
	return cfg, cfg.Validate()
}

func defaultConfig() Config {
	return Config{
		OpenBao: OpenBaoConfig{
			InstanceName: "local-openbao",
			Address:      "http://127.0.0.1:8200",
			Namespaces:   []string{"*"},
			Mounts:       []string{"*"},
		},
		Watcher: WatcherConfig{
			Type:         "auditlog",
			AuditLogPath: "./testdata/audit.log",
			HTTP: HTTPWatcherConfig{
				ListenAddress: "127.0.0.1:8080",
				Path:          "/ingress",
			},
		},
		API: APIConfig{
			Enabled:       true,
			ListenAddress: "127.0.0.1:9090",
		},
		Destination: DestinationConfig{
			Type: "dryrun",
		},
		State: StateConfig{
			Type: "sqlite",
			DSN:  "./sync-state.db",
		},
		Azure: AzureConfig{
			Auth: AzureAuthConfig{Type: "environment"},
		},
		Sync: SyncConfig{
			DryRun:      true,
			WorkerCount: 2,
		},
	}
}

func normalizeDefaults(cfg *Config) {
	if cfg.Destination.Type == "" {
		if cfg.Sync.DryRun {
			cfg.Destination.Type = "dryrun"
		} else {
			cfg.Destination.Type = "azure"
		}
	}
	if cfg.Sync.WorkerCount == 0 {
		cfg.Sync.WorkerCount = 2
	}
	if cfg.Azure.Auth.Type == "" {
		cfg.Azure.Auth.Type = "environment"
	}
	if cfg.API.Enabled && cfg.API.ListenAddress == "" {
		cfg.API.ListenAddress = "127.0.0.1:9090"
	}
}

// Validate checks that the loaded configuration is internally consistent.
func (c Config) Validate() error {
	if c.OpenBao.Address == "" {
		return errors.New("openbao.address is required")
	}
	if len(c.OpenBao.Mounts) == 0 {
		return errors.New("openbao.mounts must contain at least one mount")
	}
	if len(c.OpenBao.Namespaces) == 0 {
		return errors.New("openbao.namespaces must contain at least one namespace or *")
	}
	if c.Watcher.Type != "auditlog" && c.Watcher.Type != "http" && c.Watcher.Type != "http-device" {
		return fmt.Errorf("watcher.type %q is not supported", c.Watcher.Type)
	}
	if c.Watcher.Type == "auditlog" && c.Watcher.AuditLogPath == "" {
		return errors.New("watcher.audit_log_path is required")
	}
	if c.Watcher.Type == "http" || c.Watcher.Type == "http-device" {
		if c.Watcher.HTTP.ListenAddress == "" {
			return errors.New("watcher.http.listen_address is required")
		}
		if c.Watcher.HTTP.Path == "" || !strings.HasPrefix(c.Watcher.HTTP.Path, "/") {
			return errors.New("watcher.http.path must start with /")
		}
	}
	if c.Destination.Type != "dryrun" && c.Destination.Type != "azure" {
		return fmt.Errorf("destination.type %q is not supported", c.Destination.Type)
	}
	if c.API.Enabled && c.API.ListenAddress == "" {
		return errors.New("api.listen_address is required")
	}
	if c.Sync.WorkerCount < 1 {
		return errors.New("sync.worker_count must be greater than zero")
	}
	if c.Destination.Type == "azure" {
		if err := validateAzureAuth("azure.auth", c.Azure.Auth); err != nil {
			return err
		}
		for _, rg := range c.Azure.ResourceGroups {
			if rg.Name == "" {
				return errors.New("azure.resource_groups[].name is required")
			}
			if rg.Auth.Type != "" {
				if err := validateAzureAuth("azure.resource_groups["+rg.Name+"].auth", rg.Auth); err != nil {
					return err
				}
			}
		}
		for _, kv := range c.Azure.KeyVaults {
			if kv.Name == "" {
				return errors.New("azure.key_vaults[].name is required")
			}
			if kv.Auth.Type != "" {
				if err := validateAzureAuth("azure.key_vaults["+kv.Name+"].auth", kv.Auth); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateAzureAuth(path string, auth AzureAuthConfig) error {
	switch auth.Type {
	case "", "environment", "managed_identity":
		return nil
	case "client_secret":
		if auth.TenantID == "" {
			return fmt.Errorf("%s.tenant_id is required for client_secret auth", path)
		}
		if auth.ClientID == "" {
			return fmt.Errorf("%s.client_id is required for client_secret auth", path)
		}
		if auth.ClientSecret == "" && auth.ClientSecretEnv == "" {
			return fmt.Errorf("%s.client_secret or %s.client_secret_env is required for client_secret auth", path, path)
		}
		return nil
	default:
		return fmt.Errorf("%s.type %q is not supported", path, auth.Type)
	}
}
