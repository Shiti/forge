package embed

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rustic-ai/forge/forge-go/guild/store"
)

// StartSQLite initializes a SQLite database at the given data directory and returns a Store.
// It creates the necessary directories and the default database file (forge.db).
func StartSQLite(dataDir string) (store.Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	dbPath := filepath.Join(dataDir, "forge.db")

	s, err := store.NewGormStore(store.DriverSQLite, dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sqlite store: %w", err)
	}

	return s, nil
}
