package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvParsesEntriesAndPreservesExistingEnvironment(t *testing.T) {
	const fileKey = "CAMPUSECHO_DOTENV_FILE_TEST"
	const existingKey = "CAMPUSECHO_DOTENV_EXISTING_TEST"
	t.Setenv(existingKey, "from-environment")
	t.Setenv(fileKey, "")
	if err := os.Unsetenv(fileKey); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(fileKey)
	})

	path := filepath.Join(t.TempDir(), "test.env")
	content := "\n# comment\ninvalid line\n=ignored\n " + fileKey + " = \"value=with=equals\"\n" + existingKey + "=from-file\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	loadDotEnv(path)

	if got := os.Getenv(fileKey); got != "value=with=equals" {
		t.Fatalf("value loaded from dotenv = %q, want quoted value with equals", got)
	}
	if got := os.Getenv(existingKey); got != "from-environment" {
		t.Fatalf("existing environment value = %q, want it preserved", got)
	}

	loadDotEnv(filepath.Join(t.TempDir(), "missing.env"))
}
