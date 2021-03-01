/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package audit

import (
	"fmt"
	"testing"
)

func TestAudit(t *testing.T) {
	//	req := `{"operationName":null,"variables":{"user":"groot","pass":"password"},
	//"query":"mutation ($user: String!, $pass: String!) {\n  resetPassword(input: {userId: $user, password: $pass, namespace: 0}) {\n    userId\n  }\n}\n"}`
	//
	req := `{"operationName":null,"variables":{"user":"groot","pass":"password"},
"query":"mutation ($user: String!, $pass: String!) {\n  login(userId: $user, password: $pass) {\n    response {\n      accessJWT\n    }\n  }\n}\n"}`
	fmt.Println(maskPasswordFieldsInGQL(req))
}
