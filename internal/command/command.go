package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/icehugh/thinroute/config"

	"github.com/urfave/cli/v2"
)

var managementConfigPath string

// Run executes a thinroute management command.
func Run(args []string) error {
	app := &cli.App{
		Name:  "thinroute",
		Usage: "thinroute gateway management CLI",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", EnvVars: []string{"THINROUTE_CONFIG"}, Usage: "Path to config.yaml"},
		},
		Before: func(c *cli.Context) error {
			managementConfigPath = c.String("config")
			return nil
		},
		Commands: []*cli.Command{
			providersCmd(),
			usageCmd(),
			modelsCmd(),
			configCmd(),
		},
	}

	return app.Run(args)
}

// --- shared helpers ---

type client struct {
	baseURL string
	hc      http.Client
}

func controlBaseURL() string {
	lr, err := config.Load(managementConfigPath)
	if err != nil || lr == nil || lr.Config == nil {
		return "http://127.0.0.1:52181"
	}
	return "http://" + lr.Config.Control.Listen
}

func newClient() *client {
	return &client{
		baseURL: strings.TrimRight(controlBaseURL(), "/"),
		hc:      http.Client{Timeout: 10 * time.Second},
	}
}

func (c *client) get(path string, target any) error {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func (c *client) getRaw(path string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (c *client) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.hc.Do(req)
}

func (c *client) post(path string, body io.Reader) (*http.Response, error) {
	return c.do(http.MethodPost, path, body)
}

func (c *client) readBody(resp *http.Response) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("empty response")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return strings.TrimSpace(string(body)), err
}

// --- providers ---

type providerStatusItem struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	StatusLabel string `json:"status_label"`
	LastError   string `json:"last_error,omitempty"`
	Runtime     struct {
		DiscoveredModelCount    int        `json:"discovered_model_count"`
		LastModelFetchAt        *time.Time `json:"last_model_fetch_at,omitempty"`
		LastModelFetchSuccessAt *time.Time `json:"last_model_fetch_success_at,omitempty"`
	} `json:"runtime"`
}

type providerStatusResponse struct {
	Summary struct {
		Total     int `json:"total"`
		Healthy   int `json:"healthy"`
		Degraded  int `json:"degraded"`
		Unhealthy int `json:"unhealthy"`
	} `json:"summary"`
	Providers []providerStatusItem `json:"providers"`
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func noColor() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

func statusEmoji(status string) string {
	switch status {
	case "healthy":
		return "\033[32m🟢\033[0m"
	case "unhealthy":
		return "\033[31m🔴\033[0m"
	default:
		return "\033[33m🟡\033[0m"
	}
}

func statusLabelPlain(status string) string {
	switch status {
	case "healthy":
		return "ok"
	case "unhealthy":
		return "down"
	default:
		return "slow"
	}
}

func statusLabelRich(status string) string {
	switch status {
	case "healthy":
		return "\033[32mok\033[0m"
	case "unhealthy":
		return "\033[31mdown\033[0m"
	default:
		return "\033[33mslow\033[0m"
	}
}

func formatLastRefresh(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	d := time.Since(*t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

func providersCmd() *cli.Command {
	return &cli.Command{
		Name:  "providers",
		Usage: "Provider operations",
		Subcommands: []*cli.Command{
			{
				Name:  "status",
				Usage: "Show provider status table",
				Action: func(c *cli.Context) error {
					cl := newClient()
					var resp providerStatusResponse
					if err := cl.get("/control/v1/providers", &resp); err != nil {
						return err
					}

					color := isTTY() && !noColor()

					w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					if color {
						fmt.Fprintln(w, "NAME\tSTATUS\tLATENCY\tMODELS\tLAST REFRESH")
					} else {
						fmt.Fprintln(w, "NAME\tSTATUS\tLATENCY\tMODELS\tLAST REFRESH")
					}

					for _, p := range resp.Providers {
						models := "-"
						if p.Runtime.DiscoveredModelCount > 0 {
							models = fmt.Sprintf("%d", p.Runtime.DiscoveredModelCount)
						}

						refreshTime := p.Runtime.LastModelFetchSuccessAt
						if refreshTime == nil {
							refreshTime = p.Runtime.LastModelFetchAt
						}
						refresh := formatLastRefresh(refreshTime)

						// ponytail: latency field not available in current control API;
						// add when provider runtime snapshot exposes probe latency.
						latency := "-"

						if color {
							fmt.Fprintf(w, "%s\t%s %s\t%s\t%s\t%s\n",
								p.Name,
								statusEmoji(p.Status),
								statusLabelRich(p.Status),
								latency,
								models,
								refresh)
						} else {
							fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
								p.Name,
								statusLabelPlain(p.Status),
								latency,
								models,
								refresh)
						}
					}
					return w.Flush()
				},
			},
			{
				Name:  "test",
				Usage: "Test provider connectivity",
				Action: func(c *cli.Context) error {
					if c.NArg() != 1 {
						return fmt.Errorf("usage: thinroute providers test <name>")
					}
					cl := newClient()
					resp, err := cl.post("/control/v1/providers/"+c.Args().First()+"/test", nil)
					if err != nil {
						return err
					}
					body, err := cl.readBody(resp)
					if err != nil {
						return err
					}
					if resp.StatusCode >= 400 {
						return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
					}
					fmt.Println(body)
					return nil
				},
			},
		},
	}
}

// --- usage ---

func usageCmd() *cli.Command {
	return &cli.Command{
		Name:  "usage",
		Usage: "Show usage and cost statistics",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "Output raw JSON"},
			&cli.StringFlag{Name: "watch", Usage: "Watch mode: refresh interval (e.g. 2s)"},
			&cli.IntFlag{Name: "days", Usage: "Number of days to query (default: 1)", Value: 1},
		},
		Action: func(c *cli.Context) error {
			days := c.Int("days")
			if days <= 0 {
				days = 1
			}
			path := fmt.Sprintf("/control/v1/usage/summary?days=%d", days)
			modelsPath := fmt.Sprintf("/control/v1/usage/models?days=%d&limit=5", days)

			watch := c.String("watch")
			if watch != "" {
				d, err := time.ParseDuration(watch)
				if err != nil {
					return fmt.Errorf("invalid watch duration: %w", err)
				}
				return watchUsage(path, modelsPath, days, d)
			}

			return displayUsage(path, modelsPath, c.Bool("json"))
		},
	}
}

func displayUsage(summaryPath, modelsPath string, asJSON bool) error {
	cl := newClient()

	var summary map[string]any
	if err := cl.get(summaryPath, &summary); err != nil {
		return err
	}

	if asJSON {
		var modelsResp struct {
			Models []map[string]any `json:"models"`
		}
		cl.get(modelsPath, &modelsResp)
		out := map[string]any{
			"summary":    summary,
			"top_models": modelsResp.Models,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Println("Today's usage")
	fmt.Println("─────────────")

	requests := toFloat(summary["total_requests"])
	tokens := toFloat(summary["total_tokens"])
	inputTokens := toFloat(summary["total_input_tokens"])
	outputTokens := toFloat(summary["total_output_tokens"])
	cost := toFloat(summary["total_cost"])
	cachedTokens := toFloat(summary["cached_input_tokens"])

	fmt.Printf("  Requests:     %.0f\n", requests)
	fmt.Printf("  Tokens:       %.0f (in: %.0f, out: %.0f)\n", tokens, inputTokens, outputTokens)
	if cachedTokens > 0 {
		fmt.Printf("  Cached:       %.0f tokens\n", cachedTokens)
	}
	fmt.Printf("  Cost:         $%.4f\n", cost)

	// Top models
	var modelsResp struct {
		Models []struct {
			Model    string `json:"model"`
			Provider string `json:"provider"`
			Requests int    `json:"requests"`
			Tokens   int64  `json:"total_tokens"`
		} `json:"models"`
	}
	if err := cl.get(modelsPath, &modelsResp); err == nil && len(modelsResp.Models) > 0 {
		fmt.Println()
		fmt.Println("Top models")
		fmt.Println("──────────")
		for i, m := range modelsResp.Models {
			if i >= 5 {
				break
			}
			fmt.Printf("  %s (%s): %d reqs, %d tokens\n", m.Model, m.Provider, m.Requests, m.Tokens)
		}
	}
	return nil
}

func watchUsage(summaryPath, modelsPath string, days int, interval time.Duration) error {
	for {
		fmt.Print("\033[H\033[2J") // clear screen
		if err := displayUsage(summaryPath, modelsPath, false); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		fmt.Printf("\nRefreshing every %s. Ctrl+C to stop.\n", interval)
		time.Sleep(interval)
	}
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

// --- config ---

func configCmd() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Config file operations",
		Subcommands: []*cli.Command{
			{
				Name:  "validate",
				Usage: "Validate config.yaml",
				Action: func(c *cli.Context) error {
					if !c.Bool("strict") {
						os.Setenv("CONFIG_STRICT", "false")
						defer os.Unsetenv("CONFIG_STRICT")
					}
					_, err := config.Load(managementConfigPath)
					if err != nil {
						return fmt.Errorf("config validation failed:\n%w", err)
					}
					fmt.Println("config is valid")
					return nil
				},
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "strict", Usage: "Treat unknown fields as errors (default: true)", Value: true},
				},
			},
		},
	}
}

// --- models ---

func modelsCmd() *cli.Command {
	return &cli.Command{
		Name:  "models",
		Usage: "List available models",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List all models in the catalog",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: "Include provider and pricing info"},
				},
				Action: func(c *cli.Context) error {
					cl := newClient()
					var resp struct {
						Models []struct {
							ID       string `json:"id"`
							Provider string `json:"provider"`
							Name     string `json:"name,omitempty"`
							Pricing  *struct {
								Input  float64 `json:"input"`
								Output float64 `json:"output"`
							} `json:"pricing,omitempty"`
						} `json:"models"`
					}
					if err := cl.get("/control/v1/models", &resp); err != nil {
						return err
					}
					w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					if c.Bool("verbose") {
						fmt.Fprintln(w, "MODEL\tPROVIDER\tINPUT$/M\tOUTPUT$/M")
					} else {
						fmt.Fprintln(w, "MODEL\tPROVIDER")
					}
					for _, m := range resp.Models {
						id := m.ID
						if id == "" {
							id = m.Name
						}
						if c.Bool("verbose") && m.Pricing != nil {
							fmt.Fprintf(w, "%s\t%s\t%.4f\t%.4f\n", id, m.Provider, m.Pricing.Input, m.Pricing.Output)
						} else if c.Bool("verbose") {
							fmt.Fprintf(w, "%s\t%s\t-\t-\n", id, m.Provider)
						} else {
							fmt.Fprintf(w, "%s\t%s\n", id, m.Provider)
						}
					}
					w.Flush()
					return nil
				},
			},
		},
	}
}
