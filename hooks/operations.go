package hooks

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstallHook installs or updates a hook with a managed section.
func InstallHook(hookPath, managedSection, hookName, backupDir, markerBegin, markerEnd string, force bool) error {
	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s", hookName, timestamp))

	existingContent, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing hook: %w", err)
	}

	if len(existingContent) == 0 {
		content := "#!/bin/sh\n" + managedSection
		if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
			return fmt.Errorf("failed to write hook: %w", err)
		}
		fmt.Printf("✅ Created %s\n", hookName)
		return nil
	}

	if err := os.WriteFile(backupPath, existingContent, 0644); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	fmt.Printf("📁 Backup created: %s\n", backupPath)

	contentStr := string(existingContent)
	if strings.Contains(contentStr, markerBegin) {
		if !force {
			fmt.Printf("ℹ️  %s already has lrc section (use --force=false to skip updating)\n", hookName)
			return nil
		}
		newContent := ReplaceManagedSection(contentStr, managedSection, markerBegin, markerEnd)
		if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
			return fmt.Errorf("failed to update hook: %w", err)
		}
		fmt.Printf("✅ Updated %s (replaced lrc section)\n", hookName)
		return nil
	}

	var newContent string
	if !strings.HasPrefix(contentStr, "#!/") {
		newContent = "#!/bin/sh\n" + managedSection + "\n" + contentStr
	} else {
		lines := strings.SplitN(contentStr, "\n", 2)
		if len(lines) == 1 {
			newContent = lines[0] + "\n" + managedSection
		} else {
			newContent = lines[0] + "\n" + managedSection + "\n" + lines[1]
		}
	}

	if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}
	fmt.Printf("✅ Updated %s (added lrc section)\n", hookName)

	return nil
}

// UninstallHook removes managed section from a hook file.
func UninstallHook(hookPath, hookName, markerBegin, markerEnd string) error {
	content, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read hook: %w", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, markerBegin) {
		return nil
	}

	newContent := RemoveManagedSection(contentStr, markerBegin, markerEnd)

	trimmed := strings.TrimSpace(newContent)
	if trimmed == "" || trimmed == "#!/bin/sh" {
		if err := os.Remove(hookPath); err != nil {
			return fmt.Errorf("failed to remove hook file: %w", err)
		}
		fmt.Printf("🗑️  Removed %s (was empty after removing lrc section)\n", hookName)
		return nil
	}

	if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}
	fmt.Printf("✅ Removed lrc section from %s\n", hookName)

	return nil
}

// ReplaceManagedSection replaces one managed section in hook content.
func ReplaceManagedSection(content, newSection, markerBegin, markerEnd string) string {
	start := strings.Index(content, markerBegin)
	if start == -1 {
		return content
	}

	end := strings.Index(content[start:], markerEnd)
	if end == -1 {
		return content
	}
	end += start + len(markerEnd)

	if end < len(content) && content[end] == '\n' {
		end++
	}

	return content[:start] + newSection + "\n" + content[end:]
}

// RemoveManagedSection removes all managed sections from hook content.
func RemoveManagedSection(content, markerBegin, markerEnd string) string {
	for {
		start := strings.Index(content, markerBegin)
		if start == -1 {
			return content
		}

		end := strings.Index(content[start:], markerEnd)
		if end == -1 {
			return content
		}
		end += start + len(markerEnd)

		if end < len(content) && content[end] == '\n' {
			end++
		}

		content = content[:start] + content[end:]
	}
}

// CleanOldBackups removes old backup files, keeping only the last N.
func CleanOldBackups(backupDir string, keepLast int) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	backupsByHook := make(map[string][]os.DirEntry)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		parts := strings.SplitN(name, ".", 2)
		if len(parts) == 2 {
			hookName := parts[0]
			backupsByHook[hookName] = append(backupsByHook[hookName], entry)
		}
	}

	for hookName, backups := range backupsByHook {
		if len(backups) <= keepLast {
			continue
		}

		for i := 0; i < len(backups)-keepLast; i++ {
			oldPath := filepath.Join(backupDir, backups[i].Name())
			if err := os.Remove(oldPath); err != nil {
				log.Printf("Warning: failed to remove old backup %s: %v", oldPath, err)
			} else {
				log.Printf("Removed old backup: %s", backups[i].Name())
			}
		}
		log.Printf("Cleaned up old %s backups (kept last %d)", hookName, keepLast)
	}

	return nil
}
