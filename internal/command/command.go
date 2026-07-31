package command

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/0xfig-labs/thinroute/config"
	"github.com/0xfig-labs/thinroute/internal/core"
	"github.com/0xfig-labs/thinroute/internal/usage"
	"os"
	"time"

	"github.com/0xfig-labs/thinroute/internal/providers"
	"github.com/0xfig-labs/thinroute/run"
	"github.com/urfave/cli/v2"
)

var managementConfigPath string

// Run executes local, non-daemon management commands.
func Run(args []string) error {
	app := &cli.App{
		Name:  "thinroute",
		Usage: "thinroute local CLI",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", EnvVars: []string{"THINROUTE_CONFIG"}, Usage: "Path to config.yaml"},
		},
		Before: func(c *cli.Context) error {
			managementConfigPath = c.String("config")
			return nil
		},
		Commands: []*cli.Command{configCmd(), usageCmd(), providersCmd(), modelsCmd(), virtualModelsCmd(), doctorCmd()},
	}
	return app.Run(args)
}

func configCmd() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Config file operations",
		Subcommands: []*cli.Command{
			{
				Name:  "validate",
				Usage: "Validate config.yaml",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "strict", Usage: "Treat unknown fields as errors", Value: true},
				},
				Action: func(c *cli.Context) error {
					if !c.Bool("strict") {
						os.Setenv("CONFIG_STRICT", "false")
						defer os.Unsetenv("CONFIG_STRICT")
					}
					if _, err := config.Load(managementConfigPath); err != nil {
						return fmt.Errorf("config validation failed:\n%w", err)
					}
					fmt.Println("config is valid")
					return nil
				},
			},
		},
	}
}
func usageCmd() *cli.Command {
	return &cli.Command{
		Name:  "usage",
		Usage: "Show usage and cost statistics from SQLite",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "days", Value: 1, Usage: "Number of days to query"},
			&cli.BoolFlag{Name: "json", Usage: "Output JSON"},
		},
		Action: func(c *cli.Context) error {
			if c.Int("days") < 1 {
				return fmt.Errorf("--days must be greater than zero")
			}
			loaded, err := config.Load(managementConfigPath)
			if err != nil {
				return err
			}
			result, err := usage.New(context.Background(), loaded.Config)
			if err != nil {
				return err
			}
			defer result.Close()
			reader, err := usage.NewReader(result.Storage)
			if err != nil {
				return err
			}
			if reader == nil {
				return fmt.Errorf("usage tracking is disabled")
			}
			now := time.Now()
			params := usage.UsageQueryParams{
				StartDate: now.AddDate(0, 0, 1-c.Int("days")),
				EndDate:   now,
			}
			summary, err := reader.GetSummary(context.Background(), params)
			if err != nil {
				return err
			}
			models, err := reader.GetUsageByModel(context.Background(), params)
			if err != nil {
				return err
			}
			if c.Bool("json") {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"summary": summary,
					"models":  models,
				})
			}
			fmt.Printf("Requests: %d\nTokens: %d (in: %d, out: %d)\nCost: $%.4f\n",
				summary.TotalRequests, summary.TotalTokens, summary.TotalInput, summary.TotalOutput, valueOrZero(summary.TotalCost))
			for i, model := range models {
				if i == 5 {
					break
				}
				fmt.Printf("%s (%s): %d tokens\n",
					model.Model, model.Provider, model.InputTokens+model.OutputTokens)
			}
			return nil
		},
	}
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
func loadProviders() (*config.LoadResult, *providers.InitResult, error) {
	loaded, err := config.Load(managementConfigPath)
	if err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	result, err := providers.Init(ctx, loaded, run.DefaultProviderFactory(loaded.Config))
	if err != nil {
		return nil, nil, err
	}
	return loaded, result, nil
}

func providersCmd() *cli.Command {
	return &cli.Command{
		Name: "providers",
		Subcommands: []*cli.Command{
			{
				Name: "status",
				Action: func(c *cli.Context) error {
					_, result, err := loadProviders()
					if err != nil {
						return err
					}
					defer result.Close()
					for _, item := range result.Registry.ProviderRuntimeSnapshots() {
						fmt.Printf("%s\t%s\tmodels=%d\n", item.Name, item.Type, item.DiscoveredModelCount)
					}
					return nil
				},
			},
			{
				Name: "test",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "models", Usage: "Also fetch the provider model list"},
					&cli.BoolFlag{Name: "json", Usage: "Output JSON"},
				},
				Action: func(c *cli.Context) error {
					if c.NArg() != 1 {
						return fmt.Errorf("usage: thinroute providers test <name>")
					}
					_, result, err := loadProviders()
					if err != nil {
						return err
					}
					defer result.Close()
					name := c.Args().First()
					provider := result.Registry.ProviderByName(name)
					if provider == nil {
						return fmt.Errorf("provider %q not found", name)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					out := map[string]any{"provider": name, "available": false}
					checker, ok := provider.(core.AvailabilityChecker)
					if !ok {
						return fmt.Errorf("provider %q does not support availability checks", name)
					}
					if err := checker.CheckAvailability(ctx); err != nil {
						out["error"] = err.Error()
					} else {
						out["available"] = true
					}
					if c.Bool("models") {
						if response, err := provider.ListModels(ctx); err != nil {
							out["models_error"] = err.Error()
						} else {
							out["model_count"] = len(response.Data)
						}
					}
					if c.Bool("json") {
						return json.NewEncoder(os.Stdout).Encode(out)
					}
					if out["available"] == true {
						fmt.Printf("%s: ok\n", name)
					} else {
						fmt.Printf("%s: failed: %v\n", name, out["error"])
					}
					if count, ok := out["model_count"]; ok {
						fmt.Printf("models: %v\n", count)
					}
					return nil
				},
			},
			{
				Name: "benchmark",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "model", Required: true},
					&cli.StringFlag{Name: "prompt", Value: "Say hello briefly."},
					&cli.IntFlag{Name: "max-tokens", Value: 32},
					&cli.IntFlag{Name: "runs", Value: 1},
					&cli.BoolFlag{Name: "json"},
				},
				Action: func(c *cli.Context) error {
					_, result, err := loadProviders()
					if err != nil {
						return err
					}
					defer result.Close()
					provider := result.Registry.GetProvider(c.String("model"))
					if provider == nil {
						return fmt.Errorf("model %q not found", c.String("model"))
					}
					if c.Int("runs") < 1 || c.Int("max-tokens") < 1 {
						return fmt.Errorf("--runs and --max-tokens must be greater than zero")
					}
					type sample struct {
						ElapsedMS    int64   `json:"elapsed_ms"`
						OutputTokens int     `json:"output_tokens"`
						TokensPerSec float64 `json:"tokens_per_sec"`
					}
					samples := make([]sample, 0, c.Int("runs"))
					for i := 0; i < c.Int("runs"); i++ {
						maxTokens := c.Int("max-tokens")
						req := &core.ChatRequest{
							Model: c.String("model"), MaxTokens: &maxTokens,
							Messages: []core.Message{{Role: "user", Content: c.String("prompt")}},
						}
						start := time.Now()
						response, err := provider.ChatCompletion(context.Background(), req)
						if err != nil {
							return fmt.Errorf("run %d: %w", i+1, err)
						}
						elapsed := time.Since(start)
						tokens := 0
						if response != nil {
							tokens = response.Usage.CompletionTokens
						}
						samples = append(samples, sample{
							ElapsedMS: elapsed.Milliseconds(), OutputTokens: tokens,
							TokensPerSec: float64(tokens) / elapsed.Seconds(),
						})
					}
					if c.Bool("json") {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{"model": c.String("model"), "runs": samples})
					}
					for i, item := range samples {
						fmt.Printf("run %d: %d ms, %d output tokens, %.2f token/s\n",
							i+1, item.ElapsedMS, item.OutputTokens, item.TokensPerSec)
					}
					return nil
				},
			},
		},
	}
}

func modelsCmd() *cli.Command {
	return &cli.Command{
		Name: "models",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Flags: []cli.Flag{&cli.BoolFlag{Name: "json"}},
				Action: func(c *cli.Context) error {
					_, result, err := loadProviders()
					if err != nil {
						return err
					}
					defer result.Close()
					models := result.Registry.ListPublicModels()
					if c.Bool("json") {
						return json.NewEncoder(os.Stdout).Encode(models)
					}
					for _, model := range models {
						fmt.Printf("%s\t%s\n", model.ID, result.Registry.GetProviderType(model.ID))
					}
					return nil
				},
			},
		},
	}
}
func virtualModelsCmd() *cli.Command {
	return &cli.Command{
		Name: "virtual-models",
		Subcommands: []*cli.Command{{
			Name: "list",
			Action: func(c *cli.Context) error {
				loaded, err := config.Load(managementConfigPath)
				if err != nil {
					return err
				}
				return json.NewEncoder(os.Stdout).Encode(loaded.Config.VirtualModels)
			},
		}},
	}
}
func doctorCmd() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Flags: []cli.Flag{&cli.BoolFlag{Name: "json"}},
		Action: func(c *cli.Context) error {
			loaded, result, err := loadProviders()
			if err != nil {
				return err
			}
			defer result.Close()
			report := map[string]any{
				"config":         "ok",
				"providers":      len(result.Registry.ProviderRuntimeSnapshots()),
				"models":         result.Registry.ModelCount(),
				"virtual_models": len(loaded.Config.VirtualModels),
			}
			if c.Bool("json") {
				return json.NewEncoder(os.Stdout).Encode(report)
			}
			for key, value := range report {
				fmt.Printf("%s: %v\n", key, value)
			}
			return nil
		},
	}
}
