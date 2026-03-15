package filesystem

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const GuildGlobalScope = "GUILD_GLOBAL"

// FileSystemResolver generates and validates paths for the centralized filesystem
type FileSystemResolver struct {
	basePath string
}

func NewFileSystemResolver(basePath string) *FileSystemResolver {
	return &FileSystemResolver{basePath: basePath}
}

// ResolvePath constructs a localized filesystem path for a given guild or agent.
// agentID is optional (if empty, resolves to guild-global scope)
func (r *FileSystemResolver) ResolvePath(orgID, guildID, agentID string) string {
	if agentID == "" {
		agentID = GuildGlobalScope
	}
	parts := []string{r.basePath, orgID, guildID}
	parts = append(parts, agentID)
	// filepath.Join calls filepath.Clean implicitly
	return filepath.Join(parts...)
}

type DependencyConfig struct {
	ClassName      string
	PathBase       string
	Protocol       string
	StorageOptions map[string]any
}

type Scope struct {
	Protocol   string
	BucketURL  string
	ObjectPath string
	LocalRoot  string
}

func (r *FileSystemResolver) ResolveScope(dep DependencyConfig, orgID, guildID, agentID string) (Scope, error) {
	protocol := strings.ToLower(strings.TrimSpace(dep.Protocol))
	if protocol == "" {
		protocol = "file"
	}

	base := strings.TrimSpace(dep.PathBase)
	if base == "" {
		base = strings.TrimSpace(r.basePath)
	}
	if base == "" {
		return Scope{}, fmt.Errorf("path_base is required")
	}

	if strings.TrimSpace(orgID) == "" {
		orgID = guildID
	}
	if strings.TrimSpace(agentID) == "" {
		agentID = GuildGlobalScope
	}

	switch protocol {
	case "file":
		return resolveFileScope(base, orgID, guildID, agentID), nil
	case "s3":
		return resolveS3Scope(base, dep.StorageOptions, orgID, guildID, agentID)
	case "gcs", "gs":
		return resolveGCSScope(base, dep.StorageOptions, orgID, guildID, agentID)
	default:
		return Scope{}, fmt.Errorf("unsupported filesystem protocol: %s", protocol)
	}
}

func resolveFileScope(base, orgID, guildID, agentID string) Scope {
	root := strings.TrimSpace(strings.TrimPrefix(base, "file://"))
	root = filepath.Clean(root)

	// no_tmp_dir=1 tells gocloud.dev/blob/fileblob to create temp files inside
	// the bucket directory rather than in os.TempDir(). Without this, writes fail
	// with "invalid cross-device link" when the bucket dir and os.TempDir() are
	// on different mount points (e.g. bucket on /home, TMPDIR on /tmp).
	u := url.URL{Scheme: "file", Path: root, RawQuery: "no_tmp_dir=1"}
	return Scope{
		Protocol:   "file",
		BucketURL:  u.String(),
		ObjectPath: path.Join(orgID, guildID, agentID),
		LocalRoot:  root,
	}
}

func resolveS3Scope(base string, storageOptions map[string]any, orgID, guildID, agentID string) (Scope, error) {
	bucket, prefix, err := splitBucketAndPrefix(strings.TrimPrefix(base, "s3://"))
	if err != nil {
		return Scope{}, err
	}

	q := url.Values{}
	region := firstString(
		storageOptions,
		"region",
		"region_name",
		"aws_region",
	)
	if region == "" {
		if ck, ok := nestedMap(storageOptions, "client_kwargs"); ok {
			region = firstString(ck, "region", "region_name")
		}
	}
	if region != "" {
		q.Set("region", region)
	}

	if endpoint := firstString(storageOptions, "endpoint", "endpoint_url"); endpoint != "" {
		q.Set("endpoint", endpoint)
	}
	if ck, ok := nestedMap(storageOptions, "client_kwargs"); ok {
		if endpoint := firstString(ck, "endpoint", "endpoint_url"); endpoint != "" {
			q.Set("endpoint", endpoint)
		}
	}

	setBoolQuery(q, storageOptions, "disableSSL", "disable_ssl")
	setBoolQuery(q, storageOptions, "s3ForcePathStyle", "force_path_style")

	bURL := url.URL{Scheme: "s3", Host: bucket, RawQuery: q.Encode()}
	return Scope{
		Protocol:   "s3",
		BucketURL:  bURL.String(),
		ObjectPath: path.Join(prefix, orgID, guildID, agentID),
	}, nil
}

func resolveGCSScope(base string, storageOptions map[string]any, orgID, guildID, agentID string) (Scope, error) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(base, "gs://"), "gcs://")
	bucket, prefix, err := splitBucketAndPrefix(trimmed)
	if err != nil {
		return Scope{}, err
	}

	q := url.Values{}
	if projectID := firstString(storageOptions, "project_id", "projectID"); projectID != "" {
		q.Set("projectID", projectID)
	}
	if creds := firstString(storageOptions, "credentials_file", "credentialsFile"); creds != "" {
		q.Set("credentialsFile", creds)
	}

	bURL := url.URL{Scheme: "gs", Host: bucket, RawQuery: q.Encode()}
	return Scope{
		Protocol:   "gcs",
		BucketURL:  bURL.String(),
		ObjectPath: path.Join(prefix, orgID, guildID, agentID),
	}, nil
}

func splitBucketAndPrefix(pathBase string) (bucket string, prefix string, err error) {
	trimmed := strings.Trim(pathBase, "/")
	if trimmed == "" {
		return "", "", fmt.Errorf("path_base must include bucket name")
	}

	parts := strings.Split(trimmed, "/")
	bucket = strings.TrimSpace(parts[0])
	if bucket == "" {
		return "", "", fmt.Errorf("path_base must include bucket name")
	}
	if len(parts) > 1 {
		prefix = path.Join(parts[1:]...)
	}
	return bucket, prefix, nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if m == nil {
			return ""
		}
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func nestedMap(m map[string]any, key string) (map[string]any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	mv, ok := v.(map[string]any)
	return mv, ok
}

func setBoolQuery(q url.Values, m map[string]any, queryKey string, keys ...string) {
	for _, key := range keys {
		if m == nil {
			return
		}
		v, ok := m[key]
		if !ok {
			continue
		}
		switch b := v.(type) {
		case bool:
			q.Set(queryKey, strconv.FormatBool(b))
			return
		case string:
			if parsed, err := strconv.ParseBool(strings.TrimSpace(b)); err == nil {
				q.Set(queryKey, strconv.FormatBool(parsed))
				return
			}
		}
	}
}

// SanitizeFilename prevents directory traversal attacks for specific files
func SanitizeFilename(filename string) (string, error) {
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return "", fmt.Errorf("invalid filename")
	}
	return filename, nil
}
