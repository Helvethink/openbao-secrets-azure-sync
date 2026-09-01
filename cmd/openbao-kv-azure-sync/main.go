package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"openbao-kv-azure-sync/internal/api"
	"openbao-kv-azure-sync/internal/azure"
	"openbao-kv-azure-sync/internal/config"
	"openbao-kv-azure-sync/internal/metrics"
	"openbao-kv-azure-sync/internal/openbao"
	controller "openbao-kv-azure-sync/internal/sync"
	"openbao-kv-azure-sync/internal/watcher"
)

func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", "configs/config.example.yaml", "path to the controller configuration file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	selectedWatcher, err := newWatcher(cfg, logger)
	if err != nil {
		logger.Error("create watcher", "error", err)
		return 1
	}
	baoClient := openbao.NewClient(cfg.OpenBao.Address, cfg.OpenBao.Token, cfg.OpenBao.Namespace)
	destination, err := newDestination(cfg, logger)
	if err != nil {
		logger.Error("create destination", "error", err)
		return 1
	}

	metricsCollector := metrics.NewCollector()
	ctrl := controller.New(controller.Options{
		Watcher:        selectedWatcher,
		Source:         baoClient,
		SourceInstance: sourceInstance(cfg),
		Destination:    destination,
		Metrics:        metricsCollector,
		Logger:         logger,
	})

	type componentResult struct {
		name string
		err  error
	}
	results := make(chan componentResult, 2)
	componentCount := 1
	go func() {
		results <- componentResult{name: "controller", err: ctrl.Run(ctx)}
	}()
	if cfg.API.Enabled {
		componentCount++
		apiServer := api.NewServer(api.Options{
			ListenAddress: cfg.API.ListenAddress,
			AuthToken:     cfg.API.AuthToken,
			Controller:    ctrl,
			Metrics:       metricsCollector,
			Logger:        logger,
		})
		go func() {
			results <- componentResult{name: "api server", err: apiServer.Run(ctx)}
		}()
	}

	firstResult := <-results
	stop()
	componentResults := []componentResult{firstResult}
	for range componentCount - 1 {
		componentResults = append(componentResults, <-results)
	}
	for _, result := range componentResults {
		if result.err == nil {
			continue
		}
		logger.Error("component stopped", "component", result.name, "error", result.err)
		return 1
	}
	return 0
}

func newWatcher(cfg config.Config, logger *slog.Logger) (watcher.Watcher, error) {
	switch cfg.Watcher.Type {
	case "auditlog":
		return watcher.NewAuditLogWatcher(cfg.Watcher.AuditLogPath, cfg.OpenBao.Mounts, cfg.OpenBao.Namespaces, logger), nil
	case "http", "http-device":
		return watcher.NewHTTPDeviceWatcher(watcher.HTTPDeviceOptions{
			ListenAddress: cfg.Watcher.HTTP.ListenAddress,
			Path:          cfg.Watcher.HTTP.Path,
			AuthToken:     cfg.Watcher.HTTP.AuthToken,
			Mounts:        cfg.OpenBao.Mounts,
			Namespaces:    cfg.OpenBao.Namespaces,
			Logger:        logger,
		}), nil
	default:
		return nil, config.ErrUnsupportedWatcherType(cfg.Watcher.Type)
	}
}

func newDestination(cfg config.Config, logger *slog.Logger) (azure.Destination, error) {
	dryRunDestination := azure.NewDryRunDestination(logger)
	azureDestination, err := azure.NewKeyVaultDestination(azure.KeyVaultDestinationConfig{
		DefaultAuth:    mapAzureAuth(cfg.Azure.Auth),
		ResourceGroups: mapAzureResourceGroups(cfg.Azure.ResourceGroups),
		KeyVaults:      mapAzureKeyVaults(cfg.Azure.KeyVaults),
		Logger:         logger,
	})
	if err != nil {
		return nil, err
	}

	switch cfg.Destination.Type {
	case "dryrun":
		return azure.NewRoutingDestination(dryRunDestination, azureDestination), nil
	case "azure":
		return azure.NewRoutingDestination(dryRunDestination, azureDestination), nil
	default:
		return nil, config.ErrUnsupportedDestinationType(cfg.Destination.Type)
	}
}

func sourceInstance(cfg config.Config) string {
	if cfg.OpenBao.InstanceName != "" {
		return cfg.OpenBao.InstanceName
	}
	return cfg.OpenBao.Address
}

func mapAzureAuth(auth config.AzureAuthConfig) azure.AuthConfig {
	return azure.AuthConfig{
		Type:            auth.Type,
		TenantID:        auth.TenantID,
		ClientID:        auth.ClientID,
		ClientSecret:    auth.ClientSecret,
		ClientSecretEnv: auth.ClientSecretEnv,
	}
}

func mapAzureResourceGroups(groups []config.AzureResourceGroupConfig) []azure.ResourceGroupConfig {
	mapped := make([]azure.ResourceGroupConfig, 0, len(groups))
	for _, group := range groups {
		mapped = append(mapped, azure.ResourceGroupConfig{
			Name:           group.Name,
			SubscriptionID: group.SubscriptionID,
			Auth:           mapAzureAuth(group.Auth),
		})
	}
	return mapped
}

func mapAzureKeyVaults(vaults []config.AzureKeyVaultConfig) []azure.KeyVaultConfig {
	mapped := make([]azure.KeyVaultConfig, 0, len(vaults))
	for _, vault := range vaults {
		mapped = append(mapped, azure.KeyVaultConfig{
			Name:          vault.Name,
			URL:           vault.URL,
			ResourceGroup: vault.ResourceGroup,
			Auth:          mapAzureAuth(vault.Auth),
		})
	}
	return mapped
}
