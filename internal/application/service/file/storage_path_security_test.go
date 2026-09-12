package file

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKS3PathRejectsConfiguredBucketMismatch(t *testing.T) {
	svc := &ks3FileService{bucketName: "configured"}
	_, err := svc.parseKS3ObjectKey("ks3://other/weknora/2/secret.png")
	require.Error(t, err)
}

func TestTOSPathRejectsUnconfiguredBucket(t *testing.T) {
	svc := &tosFileService{bucketName: "main", tempBucketName: "temp"}
	_, _, _, err := svc.parseTOSPath("tos://other/weknora/2/secret.png")
	require.Error(t, err)
}

func TestOSSPathRejectsUnconfiguredBucket(t *testing.T) {
	svc := &ossFileService{bucketName: "main", tempBucketName: "temp"}
	_, _, _, err := svc.ossClientForPath("oss://other/weknora/2/secret.png")
	require.Error(t, err)
}

func TestOBSPathRejectsUnknownPrefix(t *testing.T) {
	svc := &obsFileService{bucketName: "main"}
	_, err := svc.parseObsFilePath("https://attacker.example/weknora/2/secret.png")
	require.Error(t, err)
}
