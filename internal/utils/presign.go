package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// presignPath is the URL path for presigned file access.
	presignPath = "/api/v1/files/presigned"
	// presignDefaultTTL is the default validity period for presigned URLs.
	// Kept short because the HMAC key alone authorizes cross-tenant access —
	// a leaked URL should expire before it can be widely abused. IM clients
	// typically fetch and cache images within seconds of receipt.
	presignDefaultTTL = 2 * time.Hour
)

// SystemHMACKey returns the deployment-wide HMAC key derived from
// SYSTEM_AES_KEY, or nil when it is unset or too short to be a real secret.
// Callers must treat nil as "this deployment cannot sign", not as an empty key.
func SystemHMACKey() []byte {
	key := os.Getenv("SYSTEM_AES_KEY")
	if len(key) < 16 {
		return nil
	}
	return []byte(key)
}

// getPresignKey returns the HMAC key derived from SYSTEM_AES_KEY.
// Returns nil if the key is not configured or invalid.
func getPresignKey() []byte {
	return SystemHMACKey()
}

// signPayload computes HMAC-SHA256 over the canonical payload string.
func signPayload(key []byte, filePath string, tenantID uint64, expires int64) string {
	payload := fmt.Sprintf("file_path=%s&tenant_id=%d&expires=%d", filePath, tenantID, expires)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignFileURL generates a presigned HTTP URL for accessing a storage file.
// baseURL is the external URL of the WeKnora instance (e.g. "https://weknora.example.com").
// filePath is the provider:// storage path (e.g. "local://1/abc/img.png").
// tenantID identifies the tenant that owns the file.
// ttl is how long the URL remains valid (0 uses the default presignDefaultTTL).
//
// Returns ("", error) if the signing key is not configured.
func SignFileURL(baseURL, filePath string, tenantID uint64, ttl time.Duration) (string, error) {
	key := getPresignKey()
	if key == nil {
		return "", fmt.Errorf("presign: SYSTEM_AES_KEY not configured")
	}
	if tenantID == 0 {
		return "", fmt.Errorf("presign: tenant ID is required")
	}
	if ttl <= 0 {
		ttl = presignDefaultTTL
	}
	expires := time.Now().Add(ttl).Unix()
	sig := signPayload(key, filePath, tenantID, expires)

	u, err := url.Parse(strings.TrimRight(baseURL, "/") + presignPath)
	if err != nil {
		return "", fmt.Errorf("presign: invalid base URL: %w", err)
	}
	q := u.Query()
	q.Set("file_path", filePath)
	q.Set("tenant_id", strconv.FormatUint(tenantID, 10))
	q.Set("expires", strconv.FormatInt(expires, 10))
	q.Set("sig", sig)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// VerifyFileURLSig checks the HMAC signature and expiry of a presigned URL.
// Returns true only if the signature is valid and the URL has not expired.
func VerifyFileURLSig(filePath string, tenantID uint64, expiresStr, sig string) bool {
	key := getPresignKey()
	if key == nil {
		return false
	}

	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return false
	}

	// Check expiry.
	if time.Now().Unix() > expires {
		return false
	}

	// Verify signature.
	expected := signPayload(key, filePath, tenantID, expires)
	return hmac.Equal([]byte(expected), []byte(sig))
}

// kbScopedExportsSegment is the only storage prefix served by the KB-scoped
// file proxy. Embedded wiki/chunk images land under exports/; raw knowledge
// uploads use {tenant}/{knowledgeID}/... and are served via
// /knowledge/{id}/download instead.
const kbScopedExportsSegment = "exports"

// ValidateStoragePathTenant ensures the tenant segment embedded in a provider://
// storage path matches the authenticated caller's tenant. Cross-tenant access
// for arbitrary tenant paths uses /api/v1/files/presigned with an HMAC bound to
// the resource owner; KB-scoped shared rendering uses ValidateKBScopedStoragePath.
func ValidateStoragePathTenant(filePath string, tenantID uint64) error {
	pathTenant, _, ok := parseStoragePathIdentity(filePath)
	if !ok {
		return fmt.Errorf("storage path has no canonical tenant segment")
	}
	if pathTenant == 0 {
		return fmt.Errorf("storage path has no tenant segment")
	}
	if pathTenant != tenantID {
		return fmt.Errorf("storage path workspace mismatch")
	}
	return nil
}

// ValidateKBScopedStoragePath is used by GET /knowledge-bases/:id/files. It
// requires the path to belong to the KB owner tenant and to live under the
// exports/ namespace used for embedded images (SaveBytes / multimodal output).
// This prevents borrowers with shared-KB read access from using the proxy to
// fetch arbitrary owner-tenant objects such as raw knowledge uploads.
func ValidateKBScopedStoragePath(filePath string, tenantID uint64) error {
	if err := ValidateStoragePathTenant(filePath, tenantID); err != nil {
		return err
	}
	_, exportsScoped, ok := parseStoragePathIdentity(filePath)
	if !ok || !exportsScoped {
		return fmt.Errorf("storage path is outside KB-scoped exports namespace")
	}
	return nil
}

// storageBackendScheme wraps a provider:// path with the concrete instance id:
// storage://<backendID>/<provider>://...  It is duplicated here (rather than
// reusing types.ParseStorageBackendPath) because internal/types already imports
// internal/utils, so a reverse import would create a cycle.
const storageBackendScheme = "storage://"

// unwrapStorageBackendPath strips a leading storage://<backendID>/ wrapper and
// returns the inner provider:// path. Non-wrapped paths are returned unchanged.
// This keeps tenant/exports parsing anchored on the provider path instead of
// relying on the backend id happening not to look like a tenant segment.
func unwrapStorageBackendPath(filePath string) string {
	if !strings.HasPrefix(filePath, storageBackendScheme) {
		return filePath
	}
	rest := strings.TrimPrefix(filePath, storageBackendScheme)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return filePath
	}
	return parts[1]
}

// storagePathHasExportsScope reports whether tenantID appears next to an
// exports segment in either canonical layout:
//   - {tenant}/exports/...  (local, minio, s3, most cloud backends)
//   - exports/{tenant}/...  (OSS temp-bucket layout)
//
// parseStoragePathIdentity reads the tenant from the structural tail written by
// every storage driver. Main objects end in tenant/scope-or-knowledge/file;
// temporary export objects end in exports-or-temp/tenant/file. Prefix, bucket,
// backend ID and COS region segments are deliberately ignored because each may
// legally be numeric and therefore cannot carry authorization meaning.
func parseStoragePathIdentity(filePath string) (tenantID uint64, exportsScoped bool, ok bool) {
	filePath = unwrapStorageBackendPath(strings.TrimSpace(filePath))
	if filePath == "" || strings.Contains(filePath, "\\") {
		return 0, false, false
	}
	scheme, rest, found := strings.Cut(filePath, "://")
	if !found {
		return 0, false, false
	}
	switch strings.ToLower(scheme) {
	case "local", "dummy", "minio", "s3", "cos", "tos", "oss", "ks3", "obs", "http", "https":
	default:
		return 0, false, false
	}

	rawParts := strings.Split(rest, "/")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		if part == "" {
			continue
		}
		if part == "." || part == ".." {
			return 0, false, false
		}
		parts = append(parts, part)
	}
	if len(parts) < 2 {
		return 0, false, false
	}

	parseID := func(index int) uint64 {
		if index < 0 || index >= len(parts) {
			return 0
		}
		id, err := strconv.ParseUint(parts[index], 10, 64)
		if err != nil || id == 0 {
			return 0
		}
		return id
	}

	secondFromEnd := len(parts) - 2
	thirdFromEnd := len(parts) - 3
	secondID := parseID(secondFromEnd)
	thirdID := parseID(thirdFromEnd)
	if secondID != 0 && thirdID != 0 {
		// Two adjacent numeric tail segments do not identify a unique tenant.
		return 0, false, false
	}
	if thirdID != 0 {
		return thirdID, parts[secondFromEnd] == kbScopedExportsSegment, true
	}
	if secondID != 0 {
		scope := ""
		if thirdFromEnd >= 0 {
			scope = parts[thirdFromEnd]
		}
		return secondID, scope == kbScopedExportsSegment || scope == "temp", true
	}
	return 0, false, false
}

// ParseTenantIDFromStoragePath extracts a tenant from the canonical structural
// tail of a storage path. It returns zero when the path is malformed or
// ambiguous. Authorization callers use ValidateStoragePathTenant so mismatches
// are surfaced as errors.
func ParseTenantIDFromStoragePath(filePath string) uint64 {
	tenantID, _, ok := parseStoragePathIdentity(filePath)
	if !ok {
		return 0
	}
	return tenantID
}
