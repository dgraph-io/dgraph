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

package pb

import (
	"google.golang.org/protobuf/proto"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Redact replaces sensitive string fields with "****"
func Redact(pb proto.Message) {
	m := pb.ProtoReflect()
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		// Check for custom option indicating sensitivity
		opts := fd.Options().(*descriptorpb.FieldOptions)
		if proto.GetExtension(opts, E_DataSensitive).(bool) {
			// If the field is a string type, replace the value with "****"
			if fd.Kind() == protoreflect.StringKind {
				m.Set(fd, protoreflect.ValueOfString("****"))
			}
		}
		return true
	})
}
