// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/siyuan-note/siyuan/kernel/backup"
	"github.com/siyuan-note/siyuan/kernel/util"

	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage workspace backups",
}

var backupRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Create a snapshot and upload it to enabled backup targets",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadBackupConfig(cmd)
		if err != nil {
			return err
		}
		targets, _ := cmd.Flags().GetStringSlice("target")
		engine := backup.NewEngine(util.WorkspaceDir, cfg)
		result, err := engine.Run(context.Background(), targets...)
		if err != nil {
			return err
		}
		if outputFormat == "json" {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("backup snapshot %s: %d files, %d bytes\n", result.Snapshot, result.Files, result.TotalBytes)
			for _, target := range result.Targets {
				status := "ok"
				if !target.OK {
					status = "failed: " + target.Error
				}
				fmt.Printf("  %-16s %s  uploaded=%d skipped=%d bytes=%d\n",
					target.ID, status, target.UploadedBlobs, target.SkippedBlobs, target.UploadedBytes)
			}
		}
		failed := false
		for _, target := range result.Targets {
			if !target.OK {
				failed = true
				break
			}
		}
		if failed {
			return fmt.Errorf("one or more backup targets failed")
		}
		return nil
	},
}

var backupStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the last backup result",
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := backup.LoadState(util.WorkspaceDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("no backup has run in this workspace")
				return nil
			}
			return err
		}
		if outputFormat == "json" {
			data, _ := json.MarshalIndent(state, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		fmt.Printf("updated: %s\n", state.UpdatedAt.Format("2006-01-02 15:04:05"))
		if state.Last != nil {
			fmt.Printf("last snapshot: %s (%d files, %d bytes)\n", state.Last.Snapshot, state.Last.Files, state.Last.TotalBytes)
		}
		writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(writer, "TARGET\tOK\tSNAPSHOT\tUPLOADED\tBYTES\tERROR")
		for id, target := range state.Targets {
			fmt.Fprintf(writer, "%s\t%v\t%s\t%d\t%d\t%s\n", id, target.LastOK, target.LastSnapshot,
				target.UploadedBlobs, target.UploadedBytes, target.LastError)
		}
		writer.Flush()
		return nil
	},
}

var backupTargetsCmd = &cobra.Command{
	Use:   "targets",
	Short: "List configured backup targets",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadBackupConfig(cmd)
		if err != nil {
			return err
		}
		writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(writer, "ID\tNAME\tTYPE\tENABLED\tLOCATION")
		for _, target := range cfg.Targets {
			location := ""
			switch target.Type {
			case backup.TypePG:
				location = target.PG.Schema
			case backup.TypeS3:
				location = target.S3.Bucket
			case backup.TypeWebDAV:
				location = target.WebDAV.Endpoint
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%v\t%s\n", target.ID, target.Name, target.Type, target.Enabled, location)
		}
		writer.Flush()
		return nil
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore one snapshot from a backup target",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadBackupConfig(cmd)
		if err != nil {
			return err
		}
		targetID, _ := cmd.Flags().GetString("target")
		snapshot, _ := cmd.Flags().GetString("snapshot")
		dest, _ := cmd.Flags().GetString("dest")
		force, _ := cmd.Flags().GetBool("force")
		passphrase, _ := cmd.Flags().GetString("passphrase")
		if passphrase == "" && cfg.Encryption != nil {
			passphrase = cfg.Encryption.Passphrase
		}
		if targetID == "" || snapshot == "" || dest == "" {
			return fmt.Errorf("--target, --snapshot and --dest are required")
		}
		var cfgTarget *backup.Target
		for _, target := range cfg.Targets {
			if target.ID == targetID {
				cfgTarget = target
				break
			}
		}
		if cfgTarget == nil {
			return fmt.Errorf("backup target [%s] not found", targetID)
		}
		target, err := backup.NewTarget(cfgTarget)
		if err != nil {
			return err
		}
		result, err := backup.Restore(context.Background(), target, snapshot, dest, force, passphrase)
		if err != nil {
			return err
		}
		fmt.Printf("restored snapshot %s: %d files, %d bytes -> %s\n", result.Snapshot, result.Files, result.Bytes, dest)
		return nil
	},
}

var backupSampleCmd = &cobra.Command{
	Use:   "sample",
	Short: "Print a sample conf/backup.json",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := &backup.Config{
			Enabled: true,
			Include: backup.Include{Data: true, Conf: true},
			Targets: []*backup.Target{
				{ID: "pg-1", Name: "PostgreSQL", Type: backup.TypePG, Enabled: false,
					PG: &backup.PG{Host: "127.0.0.1", Port: 5432, User: "postgres", Password: "", Database: "siyuan", SSLMode: "disable"}},
			},
		}
		data, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(data))
		return nil
	},
}

func loadBackupConfig(cmd *cobra.Command) (*backup.Config, error) {
	configPath, _ := cmd.Flags().GetString("config")
	if strings.TrimSpace(configPath) == "" {
		configPath = filepath.Join(util.ConfDir, "backup.json")
	}
	cfg, err := backup.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load backup config [%s]: %w (run `backup sample` for an example)", configPath, err)
	}
	return cfg, nil
}

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupRunCmd)
	backupCmd.AddCommand(backupStatusCmd)
	backupCmd.AddCommand(backupTargetsCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupSampleCmd)

	for _, command := range []*cobra.Command{backupRunCmd, backupStatusCmd, backupTargetsCmd, backupRestoreCmd} {
		command.Flags().String("config", "", "backup config path (default: workspace/conf/backup.json)")
	}
	backupRunCmd.Flags().StringSlice("target", nil, "run only these target IDs")
	backupRestoreCmd.Flags().String("target", "", "backup target ID")
	backupRestoreCmd.Flags().String("snapshot", "", "snapshot ID")
	backupRestoreCmd.Flags().String("dest", "", "restore destination workspace")
	backupRestoreCmd.Flags().Bool("force", false, "overwrite existing files in destination")
	backupRestoreCmd.Flags().String("passphrase", "", "backup encryption passphrase")
}
