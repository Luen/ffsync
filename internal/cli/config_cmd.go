package cli

import (
	"fmt"
	"os"

	"github.com/ffsync/ffsync/internal/config"
	"github.com/spf13/cobra"
)

// ConfigCmd returns the config command and subcommands.
func ConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}
	c.AddCommand(configInitCmd())
	c.AddCommand(configShowCmd())
	c.AddCommand(configSetCmd())
	return c
}

func configInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create config file with defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Email == "" {
				cfg.Email = "your@email.com"
			}
			if cfg.Password == "" {
				cfg.Password = ""
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Config written to %s\n", cfg.ConfigPath)
			return nil
		},
	}
}

func configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current config (sensitive values redacted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			r := cfg.Redacted()
			fmt.Println("config_path =", r.ConfigPath)
			fmt.Println("base_url =", r.BaseURL)
			fmt.Println("email =", r.Email)
			fmt.Println("password =", r.Password)
			return nil
		},
	}
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set config key (base_url, email, password)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			key, val := args[0], args[1]
			switch key {
			case "base_url":
				cfg.BaseURL = val
			case "email":
				cfg.Email = val
			case "password":
				cfg.Password = val
			default:
				return fmt.Errorf("unknown key: %s", key)
			}
			return cfg.Save()
		},
	}
}
