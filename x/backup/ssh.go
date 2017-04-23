package backup

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"golang.org/x/crypto/ssh"
)

type sshClient struct {
	addr string
	port string
	path string
	user string
	// TODO: ssh agent
	auth            string // password, certificate
	password        string
	privateKeyPath  string
	privateKeyPass  string
	checkSumMethod  string // md5, sha1, sha256
	checkSumCmdPath string
	ddCmdPath       string
}

type checkSumData struct {
	cmd  string
	size int
}

var (
	checkSumMethods = map[string]checkSumData{
		"md5":    checkSumData{"/usr/bin/md5sum", 32},
		"sha1":   checkSumData{"/usr/bin/sha1sum", 40},
		"sha256": checkSumData{"/usr/bin/sha256sum", 64},
	}
)

func (c sshClient) validateConfig() error {
	return nil
}

func (c sshClient) configure() (*ssh.ClientConfig, error) {
	var auth ssh.AuthMethod
	if c.auth == "password" {
		auth = ssh.Password(c.password)
	} else if c.auth == "certificate" {
		privKey, err := ioutil.ReadFile(c.privateKeyPath)
		if err != nil {
			return nil, err
		}

		var certKey []byte
		if block, _ := pem.Decode(privKey); block != nil {
			if x509.IsEncryptedPEMBlock(block) {
				decryptKey, err := x509.DecryptPEMBlock(block, []byte(c.privateKeyPass))
				if err != nil {
					return nil, err
				}

				privKey, err := x509.ParsePKCS1PrivateKey(decryptKey)
				if err != nil {
					return nil, err
				}

				certKey = pem.EncodeToMemory(&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privKey),
				})
			} else {
				certKey = pem.EncodeToMemory(block)
			}
		} else {
			return nil, fmt.Errorf("Invalid Private Key")
		}

		signer, err := ssh.ParsePrivateKey(certKey)
		if err != nil {
			return nil, err
		}
		auth = ssh.PublicKeys(signer)
	} else {
		return nil, fmt.Errorf("Invalid authentication method")
	}

	config := &ssh.ClientConfig{
		User: c.user,
		Auth: []ssh.AuthMethod{auth},
	}

	return config, nil
}

func (c sshClient) transferFile(client *ssh.Client, file *os.File, remoteFilePath string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil
	}

	ech := make(chan error, 1)
	defer close(ech)

	writer, err := session.StdinPipe()
	if err != nil {
		return err
	}
	go func() {
		defer writer.Close()

		copied, err := io.Copy(writer, file)
		if err != nil {
			ech <- err
			return
		}

		if copied != fileInfo.Size() {
			ech <- fmt.Errorf("Copy fails, send: %d, expected: %d", copied, fileInfo.Size())
			return
		}
		ech <- nil
	}()

	if err := <-ech; err != nil {
		return err
	}

	dd := c.ddCmdPath
	if dd == "" {
		dd = "/bin/dd"
	}

	//if err := session.Run(fmt.Sprintf("tee %s > /dev/null ", remoteFilePath)); err != nil {
	//if err := session.Run(fmt.Sprintf("cat > %s ", remoteFilePath)); err != nil {
	if err := session.Run(fmt.Sprintf("%s of=%s ", dd, remoteFilePath)); err != nil {
		return err
	}

	return nil
}

func (c sshClient) checkSum(client *ssh.Client, file *os.File, checkSumMethod string, remoteFilePath string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	checksum, has := checkSumMethods[checkSumMethod]
	if !has {
		return fmt.Errorf("Invalid checksum method: %s", checkSumMethod)
	}
	checksumCmd := c.checkSumCmdPath
	if checksumCmd == "" {
		checksumCmd = checksum.cmd
	}
	// TODO: use a param for ckechsum cmd
	result, err := session.Output(fmt.Sprintf("%s %s", checksumCmd, remoteFilePath))
	if err != nil {
		return fmt.Errorf("Checksum: %s", err.Error())
	}

	resultString := string(result[:checksum.size])
	if err := checkSum(checkSumMethod, file, resultString); err != nil {
		return err
	}
	return nil
}

func (c sshClient) upload(fileDir string, fileName string) error {
	config, err := c.configure()
	if err != nil {
		return err
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", c.addr, c.port), config)
	if err != nil {
		return err
	}
	defer client.Close()

	filePath := fmt.Sprintf("%s/%s", fileDir, fileName)
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	remoteFilePath := fmt.Sprintf("%s/%s", c.path, fileName)
	err = c.transferFile(client, file, remoteFilePath)
	if err != nil {
		return err
	}

	if c.checkSumMethod != "" {
		err = c.checkSum(client, file, c.checkSumMethod, remoteFilePath)
		if err != nil {
			return err
		}
	}

	log.Printf("File '%s' uploaded to %s:%s", filePath, c.addr, remoteFilePath)
	return nil
}
