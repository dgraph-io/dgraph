package backup

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"strings"
)

type backupClient interface {
	upload(fileDir string, fileName string) error
	validateConfig() error
}

var (
	// SSH configuration
	sshBackup         = flag.Bool("backup.ssh.on", false, "Enable ssh backup copy.")
	sshAddr           = flag.String("backup.ssh.addr", "", "Remote address.")
	sshPort           = flag.String("backup.ssh.port", "22", "Remote ssh port.")
	sshPath           = flag.String("backup.ssh.remote_path", "", "Remote backup directory.")
	sshUser           = flag.String("backup.ssh.user", "", "Remote user.")
	sshAuth           = flag.String("backup.ssh.authorization", "", "Authorization type (password, certificate).") // password, certificate
	sshPassword       = flag.String("backup.ssh.password", "", "User password.")
	sshPrivateKeyPath = flag.String("backup.ssh.key_path", "", "Private key.")
	sshPrivateKeyPass = flag.String("backup.ssh.key_pass", "", "Private key password.")
	// sshVerifyChecksum  = flag.Bool("backup.ssh.checksum", false, "Enable checksum validation after copy.")
	sshCheckSumMethod  = flag.String("backup.ssh.checksum_method", "", "Checksum method if it is needed (md5, sha1, sha256).") // md5, sha1, sha256
	sshCheckSumCmdPath = flag.String("backup.ssh.checksum_cmd_path", "", "Remote checksum path command.")
	sshDDCmdPath       = flag.String("backup.ssh.dd_cmd_path", "/bin/dd", "Remote dd path command.")

	// AWS S3 configuration
	s3Backup                   = flag.Bool("backup.ssh.s3", false, "Enable S3 backup copy.")
	s3CredentialsType          = flag.String("backup.s3.credentials", "", "")
	s3SharedCredentialFilePath = flag.String("backup.s3.credentials_shared_file_path", "", "")
	s3SharedCredentialProfile  = flag.String("backup.s3.credentials_shared_profile", "", "")
	s3AccessKeyID              = flag.String("backup.s3.access_key_id", "", "")
	s3SecretAccessKey          = flag.String("backup.s3.secret_access_key", "", "")
	s3Token                    = flag.String("backup.s3.token", "", "")
	s3Region                   = flag.String("backup.s3.region", "", "")
	s3MaxRetries               = flag.Int("backup.s3.max_retries", 3, "")
	s3BucketName               = flag.String("backup.s3.bucket", "", "")
	s3ConcurrentPartUpload     = flag.Int("backup.s3.concurrent_parts_upload", 5, "")
	s3PartSize                 = flag.Int("backup.s3.part_size", 50, "")
	s3VerifyChecksum           = flag.Bool("backup.s3.checksum", false, "")
)

var (
	clients map[string]backupClient
)

// RegisterConfiguredClients create and register the clients that are
// configured to be used when the backup process finishes
func RegisterConfiguredClients() {
	clients = make(map[string]backupClient)
	if *sshBackup {

		client := sshClient{
			addr:           *sshAddr,
			port:           *sshPort,
			path:           *sshPath,
			user:           *sshUser,
			auth:           strings.ToLower(*sshAuth),
			password:       *sshPassword,
			privateKeyPath: *sshPrivateKeyPath,
			privateKeyPass: *sshPrivateKeyPass,
			// verifyChecksum:  *sshVerifyChecksum,
			checkSumMethod:  strings.ToLower(*sshCheckSumMethod),
			checkSumCmdPath: *sshCheckSumCmdPath,
			ddCmdPath:       *sshDDCmdPath,
		}

		registerClient("ssh", client)

		// if err := client.ValidateConfig(); err != nil {
		// 	log.Printf("Invalid configuration for ssh backup client: %s", err.Error())
		// } else {
		// 	clients["ssh"] = client
		// }
	}

	if *s3Backup {
		client := s3Client{
			credentialsType:          strings.ToLower(*s3CredentialsType),
			sharedCredentialFilePath: *s3SharedCredentialFilePath,
			sharedCredentialProfile:  *s3SharedCredentialProfile,
			accessKeyID:              *s3AccessKeyID,
			secretAccessKey:          *s3SecretAccessKey,
			token:                    *s3Token,
			region:                   *s3Token,
			maxRetries:               *s3MaxRetries,
			bucketName:               *s3BucketName,
			concurrentPartUpload:     *s3ConcurrentPartUpload,
			partSize:                 *s3PartSize,
			verifyChecksum:           *s3VerifyChecksum,
		}

		registerClient("S3", client)
	}
}

func registerClient(name string, client backupClient) {
	if err := client.validateConfig(); err != nil {
		log.Printf("Invalid configuration for '%s' backup client: %s", name, err.Error())
		return
	}
	clients[name] = client
}

// UploadBackup executes all the configured backup methods
func UploadBackup(fileDir string, fileName string) {
	filePath := fmt.Sprintf("%s/%s", fileDir, fileName)
	for k, c := range clients {
		log.Printf("Starting '%s' upload for file '%s'", k, filePath)
		err := c.upload(fileDir, fileName)
		if err != nil {
			log.Printf("Upload fails: file '%s' using '%s': %s", filePath, k, err.Error())
		}
	}
}

func checkSum(method string, file *os.File, expected string) error {
	if file == nil {
		return fmt.Errorf("Invalid file")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}

	var h hash.Hash
	if method == "md5" {
		h = md5.New()
	} else if method == "sha1" {
		h = sha1.New()
	} else if method == "sha256" {
		h = sha256.New()
	} else {
		return fmt.Errorf("Invalid hash method")
	}

	if _, err := io.Copy(h, file); err != nil {
		return err
	}

	sum := hex.EncodeToString(h.Sum(nil))
	if expected != sum {
		return fmt.Errorf("Checksum mismatch, got: %s, expected: %s", sum, expected)
	}
	return nil
}
