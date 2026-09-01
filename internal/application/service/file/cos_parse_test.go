package file

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCosObjectName_RejectsLocalScheme(t *testing.T) {
	svc := &cosFileService{bucketName: "b", region: "ap-shanghai", bucketURL: "https://b.cos.ap-shanghai.myqcloud.com/"}
	_, err := svc.parseCosObjectName("local://10000/exports/img.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local")
}

func TestParseCosObjectName_CosScheme(t *testing.T) {
	svc := &cosFileService{
		bucketName: "bucket",
		region:     "ap-shanghai",
		bucketURL:  "https://bucket.cos.ap-shanghai.myqcloud.com/",
	}
	key, err := svc.parseCosObjectName("cos://bucket/ap-shanghai/weknora/10000/exports/a.png")
	require.NoError(t, err)
	assert.Equal(t, "weknora/10000/exports/a.png", key)
}

func TestParseCosObjectName_RejectsMinioScheme(t *testing.T) {
	svc := &cosFileService{bucketName: "b", region: "ap-shanghai", bucketURL: "https://b.cos.ap-shanghai.myqcloud.com/"}
	_, err := svc.parseCosObjectName("minio://wizard-test/10000/exports/img.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "minio")
}

func TestParseCosObjectName_RejectsBucketAndRegionMismatch(t *testing.T) {
	svc := &cosFileService{bucketName: "configured", region: "ap-shanghai"}
	_, err := svc.parseCosObjectName("cos://other/ap-shanghai/weknora/2/secret.png")
	require.Error(t, err)
	_, err = svc.parseCosObjectName("cos://configured/ap-beijing/weknora/2/secret.png")
	require.Error(t, err)
}

func TestParseCosObjectName_RejectsUnknownLegacyURL(t *testing.T) {
	svc := &cosFileService{
		bucketName: "configured", region: "ap-shanghai",
		bucketURL: "https://configured.cos.ap-shanghai.myqcloud.com/",
	}
	_, err := svc.parseCosObjectName("https://other.cos.ap-shanghai.myqcloud.com/weknora/2/secret.png")
	require.Error(t, err)
}
