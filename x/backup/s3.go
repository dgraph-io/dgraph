package backup

import (
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type credentialType uint8

const (
	// S3StaticCredentials credentialType = iota
	// S3EnvCredentials
	// S3SharedCredentials
	s3StaticCredentials = "static"
	s3EnvCredentials    = "environment"
	s3SharedCredentials = "shared"
)

type s3Client struct {
	credentialsType string
	// Shared credentials
	sharedCredentialFilePath string
	sharedCredentialProfile  string
	// Static credentials
	accessKeyID     string
	secretAccessKey string
	token           string
	// Bucket configuration
	region     string
	maxRetries int
	bucketName string
	// Upload file configuration
	concurrentPartUpload int
	partSize             int // in MB
	// Integrity check
	verifyChecksum bool
}

func (s s3Client) validateConfig() error {
	return nil
}

func (s s3Client) upload(fileDir string, fileName string) error {
	filePath := fmt.Sprintf("%s/%s", fileDir, fileName)
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	config := aws.NewConfig()
	config.Region = &s.region
	config.MaxRetries = &s.maxRetries

	// if s.credentialsType == S3StaticCredentials {
	// 	config.Credentials = credentials.NewStaticCredentials(s.accessKeyID, s.secretAccessKey, s.token)
	// } else if s.credentialsType == S3EnvCredentials {
	// 	config.Credentials = credentials.NewEnvCredentials()
	// } else if s.credentialsType == S3SharedCredentials {
	// 	config.Credentials = credentials.NewSharedCredentials(s.sharedCredentialFilePath, s.sharedCredentialProfile)
	// }

	if s.credentialsType == s3StaticCredentials {
		config.Credentials = credentials.NewStaticCredentials(s.accessKeyID, s.secretAccessKey, s.token)
	} else if s.credentialsType == s3EnvCredentials {
		config.Credentials = credentials.NewEnvCredentials()
	} else if s.credentialsType == s3SharedCredentials {
		config.Credentials = credentials.NewSharedCredentials(s.sharedCredentialFilePath, s.sharedCredentialProfile)
	}

	sess, err := session.NewSession(config)
	if err != nil {
		return err
	}

	uploader := s3manager.NewUploader(sess, func(up *s3manager.Uploader) {
		up.Concurrency = s.concurrentPartUpload
		up.PartSize = int64(s.partSize) * 1024 * 1024
		//up.RequestOptions = []request.Option{}
	})

	out, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: &s.bucketName,
		Key:    &s.bucketName,
		Body:   file,
	})
	if err != nil {
		return err
	}

	if s.verifyChecksum {
		srv := s3.New(sess)
		obj, err := srv.HeadObject(&s3.HeadObjectInput{
			Bucket: &s.bucketName,
			Key:    &s.bucketName,
		})
		if err != nil {
			return err
		}

		md5, ok := obj.Metadata["md5chksum"]
		if !ok {
			return fmt.Errorf("Cannot get checksum from S3 bucket for file '%s'", fileName)
		}
		if err := checkSum("md5", file, *md5); err != nil {
			return err
		}
	}

	log.Printf("File '%s' uploaded to %s with UploadID %s", filePath, out.Location, out.UploadID)
	return nil
}
