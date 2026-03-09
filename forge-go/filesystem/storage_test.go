package filesystem

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalFileStore(t *testing.T) {
	basePath := t.TempDir()
	resolver := NewFileSystemResolver(basePath)
	store := NewLocalFileStore(resolver)

	// Context for path
	orgID := "org-1"
	guildID := "guild-abcd"
	cfg := DependencyConfig{
		PathBase:       basePath,
		Protocol:       "file",
		StorageOptions: map[string]any{"auto_mkdir": true},
	}

	resolvedPath := resolver.ResolvePath(orgID, guildID, "")
	expectedPath := filepath.Join(basePath, orgID, guildID, GuildGlobalScope)
	assert.Equal(t, expectedPath, resolvedPath)

	// 1. Upload Test
	content := []byte("hello world")
	err := store.Upload(context.Background(), cfg, orgID, guildID, "", "test.txt", content, "text/plain", map[string]any{"category": "test"})
	require.NoError(t, err)

	// 2. List Test
	files, err := store.List(context.Background(), cfg, orgID, guildID, "")
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "test.txt", files[0].Filename)
	assert.Equal(t, "text/plain", files[0].MimeType)
	assert.Equal(t, "test", files[0].Metadata["category"])

	// 3. Download Test
	downloadedContent, err := store.Read(context.Background(), cfg, orgID, guildID, "", "test.txt")
	require.NoError(t, err)
	assert.Equal(t, content, downloadedContent)

	// 4. Sanity Test - Directory Traversal Prevention
	err = store.Upload(context.Background(), cfg, orgID, guildID, "", "../evil.txt", content, "text/plain", map[string]any{})
	assert.ErrorContains(t, err, "invalid filename")

	// 5. Delete Test
	err = store.Delete(context.Background(), cfg, orgID, guildID, "", "test.txt")
	require.NoError(t, err)

	filesAfter, _ := store.List(context.Background(), cfg, orgID, guildID, "")
	assert.Len(t, filesAfter, 0)
}
