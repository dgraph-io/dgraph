/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

// User object to denote basic demo object.
var User = GraphQLObject{
	Name: "User",
	Desc: "User object in the system",
	Fields: FieldMap{
		"uid": &Field{
			Type:    String,
			Resolve: func(rp ResolveParams) interface{} { return "uid" },
		},
		"xid": &Field{
			Type:    String,
			Resolve: func(rp ResolveParams) interface{} { return "xid" },
		},
		"age": &Field{
			Type:    Int,
			Resolve: func(rp ResolveParams) interface{} { return 1 },
		},
		"hometown": &Field{
			Type:    String,
			Resolve: func(rp ResolveParams) interface{} { return "hometown_name" },
		},
	},
}
