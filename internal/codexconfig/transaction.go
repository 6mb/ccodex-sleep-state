package codexconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gylive/ccodex-sleep-state/internal/fsutil"
)

type receipt struct {
	Target    string `json:"target"`
	Backup    string `json:"backup"`
	Existed   bool   `json:"existed"`
	Before    string `json:"before_sha256"`
	Installed string `json:"installed_sha256"`
}

func digest(b []byte) string    { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func journal(dir string) string { return filepath.Join(dir, "config-transaction.json") }

func Install(dir, codexHome, baseURL string) error {
	return InstallWithOptions(dir, codexHome, baseURL, Options{})
}

func InstallWithOptions(dir, codexHome, baseURL string, options Options) error {
	if _, err := os.Stat(journal(dir)); err == nil {
		return errors.New("unfinished config transaction; run restore before starting again")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(codexHome, 0700); err != nil {
		return err
	}
	realHome, err := filepath.EvalSymlinks(codexHome)
	if err != nil {
		return err
	}
	target := filepath.Join(realHome, "config.toml")
	if err = fsutil.RefuseLink(target); err != nil {
		return err
	}
	original, err := os.ReadFile(target)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updated, err := PatchWithOptions(original, baseURL, options)
	if err != nil {
		return err
	}
	backup, err := os.CreateTemp(realHome, "config.toml.sleep-state-"+time.Now().UTC().Format("20060102T150405")+"-*.bak")
	if err != nil {
		return err
	}
	if err = backup.Chmod(0600); err == nil {
		_, err = backup.Write(original)
	}
	if err == nil {
		err = backup.Sync()
	}
	err = errors.Join(err, backup.Close())
	if err != nil {
		return err
	}
	record := receipt{target, backup.Name(), existed, digest(original), digest(updated)}
	data, _ := json.MarshalIndent(record, "", "  ")
	if err = fsutil.Write(journal(dir), data); err != nil {
		return err
	}
	// Detect edits made during backup creation. Never clobber them.
	current, readErr := os.ReadFile(target)
	if (readErr != nil && !os.IsNotExist(readErr)) || (readErr == nil) != existed || digest(current) != record.Before {
		return errors.New("Codex config changed during setup; left unchanged, backup retained")
	}
	return fsutil.Write(target, updated)
}

func Restore(dir string) error {
	data, err := os.ReadFile(journal(dir))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var r receipt
	if json.Unmarshal(data, &r) != nil || r.Target == "" || filepath.Dir(r.Target) != filepath.Dir(r.Backup) {
		return errors.New("invalid config transaction; manual recovery required")
	}
	if err = fsutil.RefuseLink(r.Target); err != nil {
		return err
	}
	current, readErr := os.ReadFile(r.Target)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	if digest(current) == r.Before && (readErr == nil) == r.Existed {
		return os.Remove(journal(dir))
	}
	if readErr != nil || digest(current) != r.Installed {
		return errors.New("Codex config changed since setup; refusing to overwrite it; backup and transaction retained")
	}
	original, err := os.ReadFile(r.Backup)
	if err != nil {
		return err
	}
	if digest(original) != r.Before {
		return errors.New("backup checksum mismatch; refusing to restore")
	}
	if r.Existed {
		err = fsutil.Write(r.Target, original)
	} else {
		err = os.Remove(r.Target)
	}
	if err != nil {
		return err
	}
	return os.Remove(journal(dir))
}

// CheckManaged is a read-only guard for a running service. A configuration
// manager switching providers requires a fresh attach, not an automatic rewrite.
func CheckManaged(dir string) error {
	data, err := os.ReadFile(journal(dir))
	if err != nil {
		return errors.New("managed Codex configuration receipt is unavailable; reconnect Codex")
	}
	var r receipt
	if json.Unmarshal(data, &r) != nil || r.Target == "" || r.Installed == "" {
		return errors.New("invalid managed Codex configuration receipt")
	}
	if err = fsutil.RefuseLink(r.Target); err != nil {
		return err
	}
	current, err := os.ReadFile(r.Target)
	if err != nil || digest(current) != r.Installed {
		return errors.New("Codex configuration changed outside this service; reconnect after switching providers")
	}
	return nil
}
