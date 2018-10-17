/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package types

import "testing"

func TestEncrypt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:    "case 1",
			input:   "",
			want:    0,
			wantErr: true,
		},
		{
			name:    "case 2",
			input:   "12345",
			want:    0,
			wantErr: true,
		},
		{
			name:    "case 3",
			input:   "123456",
			want:    60,
			wantErr: false,
		},
		{
			name:    "case 4",
			input:   "1234567890",
			want:    60,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Encrypt(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Encrypt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.want {
				t.Errorf("Encrypt() = %v, want %v", len(got), tt.want)
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	type args struct {
		plain     string
		encrypted string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "case 1",
			args: args{
				plain:     "",
				encrypted: "",
			},
			wantErr: true,
		},
		{
			name: "case 2",
			args: args{
				plain:     "",
				encrypted: "$2a$10$LMGgvb5dOq4/YrWXjAy6W.tBfFQC4QDNFAuOCWGRk3f/Z1TMXswaC",
			},
			wantErr: true,
		},
		{
			name: "case 3",
			args: args{
				plain:     "1234567890",
				encrypted: "",
			},
			wantErr: true,
		},
		{
			name: "case 4",
			args: args{
				plain:     "12345",
				encrypted: "$2a$10$LMGgvb5dOq4/YrWXjAy6W.tBfFQC4QDNFAuOCWGRk3f/Z1TMXswaC",
			},
			wantErr: true,
		},
		{
			name: "case 5",
			args: args{
				plain:     "12345678",
				encrypted: "$2a$10$LMGgvb5dOq4/YrWXjAy6W.tBfFQC4QDNFAuOCWGRk3f/Z1TMXswaC",
			},
			wantErr: true,
		},
		{
			name: "case 6",
			args: args{
				plain:     "123456",
				encrypted: "$2a$10$LMGgvb5dOq4/YrWXjAy6W.tBfFQC4QDNFAuOCWGRk3f/Z1TMXswaC",
			},
			wantErr: false,
		},
		{
			name: "case 7",
			args: args{
				plain:     "123456",
				encrypted: "$2a$10$kXtSFCVmzsu0lwVqaWxo5OXlLGUakcY2t18QcqcVpvoTHsPqclOca",
			},
			wantErr: false,
		},
		{
			name: "case 8",
			args: args{
				plain:     "1234567890",
				encrypted: "$2a$10$WdCWNpOP6c4l7ECv3hEWKeD3oSiszlRJFmT4uRT1P/W9V9zUye8pS",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := VerifyPassword(tt.args.plain, tt.args.encrypted); (err != nil) != tt.wantErr {
				t.Errorf("VerifyPassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
