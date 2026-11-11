package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Patchwork configuration",
		Long:  `View, set, and edit configuration settings for Patchwork, including default flags, disabled package managers, and pinned packages.`,
	}

	cmd.AddCommand(
		newConfigGetCmd(),
		newConfigSetCmd(),
		newConfigListCmd(),
		newConfigEditCmd(),
	)

	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "get <key>",
		Short:     "Get the value of a configuration key",
		Long:      fmt.Sprintf(`Display the current value for a given configuration setting. Supported keys: %s.`, strings.Join(config.SupportedKeys, ", ")),
		Args:      cobra.ExactArgs(1),
		ValidArgs: config.SupportedKeys,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			val, err := cfg.Get(args[0])
			if err != nil {
				return err
			}
			if val == nil {
				cmd.Println("<not set>")
				return nil
			}
			switch v := val.(type) {
			case string:
				if v == "" {
					cmd.Println("<not set>")
				} else {
					cmd.Println(v)
				}
			case bool:
				cmd.Println(v)
			default:
				data, err := json.MarshalIndent(v, "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
			}
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "set <key> <value>",
		Short:     "Set the value of a configuration key",
		Long:      fmt.Sprintf(`Set a configuration setting persistently. Supported keys: %s.`, strings.Join(config.SupportedKeys, ", ")),
		Args:      cobra.ExactArgs(2),
		ValidArgs: config.SupportedKeys,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			if err := cfg.Set(args[0], args[1]); err != nil {
				return err
			}
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			cmd.Printf("Set %s = %s\n", args[0], args[1])
			return nil
		},
	}
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"show"},
		Short:   "Display full configuration",
		Long:    `Display the entire Patchwork configuration formatted as JSON.`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		},
	}
}

func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open configuration file in text editor",
		Long:  `Open the Patchwork configuration file in $EDITOR, $VISUAL, or default system editor.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.ConfigPath()
			if err != nil {
				return fmt.Errorf("failed to resolve config path: %w", err)
			}
			if _, err := os.Stat(path); os.IsNotExist(err) {
				emptyCfg := &config.Config{}
				if err := emptyCfg.SaveTo(path); err != nil {
					return fmt.Errorf("failed to create config file: %w", err)
				}
			}

			stdinFile, ok := cmd.InOrStdin().(*os.File)
			if !ok || !term.IsTerminal(int(stdinFile.Fd())) {
				return fmt.Errorf("cannot open editor in non-interactive environment (config file is at: %s)", path)
			}

			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = os.Getenv("VISUAL")
			}
			if editor == "" {
				if runtime.GOOS == "windows" {
					editor = "notepad"
				} else {
					editor = "nano"
				}
			}

			editorCmd := exec.Command(editor, path)
			editorCmd.Stdin = cmd.InOrStdin()
			editorCmd.Stdout = cmd.OutOrStdout()
			editorCmd.Stderr = cmd.ErrOrStderr()

			if err := editorCmd.Run(); err != nil {
				return fmt.Errorf("editor execution failed: %w", err)
			}

			if _, err := config.LoadFrom(path); err != nil {
				return fmt.Errorf("warning: saved configuration file contains invalid JSON: %w", err)
			}
			cmd.Println("Configuration updated successfully.")
			return nil
		},
	}
}
