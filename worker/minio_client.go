package worker

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	minio "github.com/minio/minio-go/v6"
	"github.com/minio/minio-go/v6/pkg/credentials"
	"github.com/minio/minio-go/v6/pkg/s3utils"
	"github.com/pkg/errors"
)

const (
	// Shown in transfer logs
	appName = "Dgraph"

	// defaultEndpointS3 is used with s3 scheme when no host is provided
	defaultEndpointS3 = "s3.amazonaws.com"

	// s3AccelerateSubstr S3 acceleration is enabled if the S3 host is contains this substring.
	// See http://docs.aws.amazon.com/AmazonS3/latest/dev/transfer-acceleration.html
	s3AccelerateSubstr = "s3-accelerate"
)

// Credentials holds the credentials needed to perform a backup/export operation.
// If these credentials are missing the default credentials will be used.
type Credentials struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
	Anonymous    bool
}

func (creds *Credentials) isAnonymous() bool {
	if creds == nil {
		return false
	}
	return creds.Anonymous
}

func credentialsProvider(scheme string, requestCreds credentials.Value) credentials.Provider {
	providers := []credentials.Provider{&credentials.Static{Value: requestCreds}}

	switch scheme {
	case "s3":
		providers = append(providers, &credentials.EnvAWS{}, &credentials.IAM{Client: &http.Client{}})
	default:
		providers = append(providers, &credentials.EnvMinio{})
	}

	return &credentials.Chain{Providers: providers}
}

func requestCreds(creds *Credentials) credentials.Value {
	if creds == nil {
		return credentials.Value{}
	}

	return credentials.Value{
		AccessKeyID:     creds.AccessKey,
		SecretAccessKey: creds.SecretKey,
		SessionToken:    creds.SessionToken,
	}
}

func newMinioClient(uri *url.URL, creds *Credentials) (*minio.Client, error) {
	if len(uri.Path) < 1 {
		return nil, errors.Errorf("Invalid bucket: %q", uri.Path)
	}

	glog.V(2).Infof("Backup/Export using host: %s, path: %s", uri.Host, uri.Path)

	// Verify URI and set default S3 host if needed.
	switch uri.Scheme {
	case "s3":
		// s3:///bucket/folder
		if !strings.Contains(uri.Host, ".") {
			uri.Host = defaultEndpointS3
		}
		if !s3utils.IsAmazonEndpoint(*uri) {
			return nil, errors.Errorf("Invalid S3 endpoint %q", uri.Host)
		}
	default: // minio
		if uri.Host == "" {
			return nil, errors.Errorf("Minio handler requires a host")
		}
	}

	secure := uri.Query().Get("secure") != "false" // secure by default

	if creds.isAnonymous() {
		return minio.New(uri.Host, "", "", secure)
	}

	credsProvider := credentials.New(credentialsProvider(uri.Scheme, requestCreds(creds)))

	mc, err := minio.NewWithCredentials(uri.Host, credsProvider, secure, "")

	if err != nil {
		return nil, err
	}

	// Set client app name "Dgraph/v1.0.x"
	mc.SetAppInfo(appName, x.Version())

	// S3 transfer acceleration support.
	if uri.Scheme == "s3" && strings.Contains(uri.Host, s3AccelerateSubstr) {
		mc.SetS3TransferAccelerate(uri.Host)
	}

	// enable HTTP tracing
	if uri.Query().Get("trace") == "true" {
		mc.TraceOn(os.Stderr)
	}

	return mc, nil
}

func validateBucket(mc *minio.Client, uri *url.URL) (string, string, error) {
	// split path into bucketName and blobPrefix
	parts := strings.Split(uri.Path[1:], "/")
	bucketName := parts[0] // bucket
	objectPrefix := ""

	glog.Info("Verifying Bucket Exists: ", bucketName)
	// verify the requested bucket exists.
	found, err := mc.BucketExists(bucketName)
	if err != nil {
		return "", "", errors.Wrapf(err, "while looking for bucket %s at host %s", bucketName, uri.Host)
	}
	if !found {
		return "", "", errors.Errorf("Bucket was not found: %s", bucketName)
	}
	if len(parts) > 1 {
		objectPrefix = filepath.Join(parts[1:]...)
	}

	return bucketName, objectPrefix, nil
}
