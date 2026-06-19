package hooks

import (
	"fmt"
	"path/filepath"

	"github.com/HexmosTech/git-lrc/storage"
)

// WriteManagedHookScripts writes all lrc-managed hook scripts into dir.
func WriteManagedHookScripts(dir string, cfg TemplateConfig) error {
	if err := storage.EnsureManagedHooksDir(dir); err != nil {
		return err
	}

	scripts := map[string]string{
		"pre-commit":         GeneratePreCommitHook(cfg),
		"prepare-commit-msg": GeneratePrepareCommitMsgHook(cfg),
		"commit-msg":         GenerateCommitMsgHook(cfg),
		"post-commit":        GeneratePostCommitHook(cfg),
		"pre-push":           GeneratePrePushHook(cfg),
	}

	for name, content := range scripts {
		path := filepath.Join(dir, name)
		script := "#!/bin/sh\n" + content
		if err := storage.WriteFile(path, []byte(script), 0755); err != nil {
			return fmt.Errorf("failed to write managed hook %s: %w", name, err)
		}
	}

	return nil
}
