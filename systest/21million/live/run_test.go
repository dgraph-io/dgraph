//go:build integration

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bulk

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/v24/systest/21million/common"
	"github.com/dgraph-io/dgraph/v24/testutil"
)

func TestQueries(t *testing.T) {
	t.Run("Run queries", common.TestQueriesFor21Million)
}

func TestMain(m *testing.M) {
	schemaFile := filepath.Join(testutil.TestDataDirectory, "21million.schema")
	rdfFile := filepath.Join(testutil.TestDataDirectory, "21million.rdf.gz")
	if err := testutil.LiveLoad(testutil.LiveOpts{
		Alpha:      testutil.ContainerAddr("alpha1", 9080),
		Zero:       testutil.SockAddrZero,
		RdfFile:    rdfFile,
		SchemaFile: schemaFile,
	}); err != nil {
		cleanupAndExit(1)
	}

	exitCode := m.Run()

	if exitCode != 0 {
		now := time.Now().Unix()
		uploadLogsToS3(now)
		uploadPDirToS3(now)
	}

	cleanupAndExit(exitCode)
}

func cleanupAndExit(exitCode int) {
	_ = os.RemoveAll("./t")
	os.Exit(exitCode)
}

func uploadLogsToS3(now int64) {
	log.Printf("getting alpha logs from docker")
	alphaLogs, err := testutil.GetContainerLogs("alpha1")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("alpha1-%v.log", alphaLogs, 0644); err != nil {
		panic(err)
	}

	log.Printf("uploading alpha1-%v.log to s3", now)
	if err := uploadDataToS3(fmt.Sprintf("alpha1-%v.log", now)); err != nil {
		panic(err)
	}
	log.Printf("uploading alpha1-%v.log to s3 completed", now)
}

func uploadPDirToS3(now int64) {
	destFile := fmt.Sprintf("p-%v.tar.gz", now)

	log.Printf("copying data from alpha1:/data/alpha1/p to %v", destFile)
	if err := testutil.CopyPDir("alpha1", "/data/alpha1/p", destFile); err != nil {
		panic(err)
	}

	log.Printf("uploading %v to S3", destFile)
	if err := uploadDataToS3(destFile); err != nil {
		panic(err)
	}
	log.Printf("uploading %v to S3 completed", destFile)
}

func uploadDataToS3(srcFile string) error {
	file, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("error opening file %v: %v", srcFile, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			glog.Errorf("error closing file %v: %v", srcFile, err)
		}
	}()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"),
	})
	if err != nil {
		return fmt.Errorf("error creating AWS session: %v", err)
	}
	s3Client := s3.New(sess)

	createResp, err := s3Client.CreateMultipartUpload(&s3.CreateMultipartUploadInput{
		Bucket: aws.String("dgraph-load-test-failures-debugging-p-dir"),
		Key:    aws.String(srcFile),
	})
	if err != nil {
		return fmt.Errorf("error creating multipart upload: %v", err)
	}

	const partSize = 5 * 1024 * 1024 // 5MB
	buffer := make([]byte, partSize)
	var completedParts []*s3.CompletedPart
	partNumber := int64(1)
	for {
		n, err := file.Read(buffer)
		if err != nil && err.Error() != "EOF" {
			return fmt.Errorf("error reading file %v: %v", srcFile, err)
		}
		if n == 0 {
			break
		}

		uploadResp, err := s3Client.UploadPart(&s3.UploadPartInput{
			Bucket:     aws.String("dgraph-load-test-failures-debugging-p-dir"),
			Key:        aws.String(srcFile),
			PartNumber: aws.Int64(partNumber),
			UploadId:   createResp.UploadId,
			Body:       bytes.NewReader(buffer[:n]),
		})
		if err != nil {
			return fmt.Errorf("error uploading part %v: %v", partNumber, err)
		}

		completedParts = append(completedParts, &s3.CompletedPart{
			ETag:       uploadResp.ETag,
			PartNumber: aws.Int64(partNumber),
		})
		partNumber++
	}

	_, err = s3Client.CompleteMultipartUpload(&s3.CompleteMultipartUploadInput{
		Bucket:          aws.String("dgraph-load-test-failures-debugging-p-dir"),
		Key:             aws.String(srcFile),
		UploadId:        createResp.UploadId,
		MultipartUpload: &s3.CompletedMultipartUpload{Parts: completedParts},
	})
	if err != nil {
		return fmt.Errorf("error completing multipart upload: %v", err)
	}

	return nil
}
