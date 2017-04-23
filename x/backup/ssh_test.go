package backup

import (
	"reflect"
	"testing"

	"golang.org/x/crypto/ssh"
)

var (
	sshClientConfig sshClient
)

func init() {
	sshClientConfig = sshClient{
		addr:           "192.168.56.101",
		port:           "22",
		path:           "/tmp",
		user:           "root",
		auth:           "password",
		password:       "",
		verifyCheckSum: true,
		checkSumMethod: "sha256",
	}
}

func Test_scpConfig_configure(t *testing.T) {
	tests := []struct {
		name    string
		c       sshClient
		want    *ssh.ClientConfig
		wantErr bool
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.configure()
			if (err != nil) != tt.wantErr {
				t.Errorf("scpConfig.configure() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("scpConfig.configure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_scpConfig_transfer(t *testing.T) {
	type args struct {
		fileName string
		fileDir  string
		checksum string
	}
	tests := []struct {
		name    string
		c       sshClient
		args    args
		wantErr bool
	}{
		{
			name: "test 1",
			c:    sshClientConfig,
			args: args{
				fileName: "client.go",
				fileDir:  "./",
				checksum: "123",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.transfer(tt.args.fileName, tt.args.fileDir); (err != nil) != tt.wantErr {
				t.Errorf("scpConfig.transfer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
