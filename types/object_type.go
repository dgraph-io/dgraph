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

// personType for validating coercion system implementation
// Resolve field is a redundant dummy for now
// TODO(akhil): Recursive definition like Person->friend(personType) throws err, figure out solution.
var personType = GraphQLObject{
	Name: "Person",
	Desc: "object to represent a person type",
	Fields: FieldMap{
		"name": &Field{
			Type: String,
			Resolve: func(rp ResolveParams) interface{} {
				return "name_person"
			},
		},
		"gender": &Field{
			Type: String,
			Resolve: func(rp ResolveParams) interface{} {
				return "gender_person"
			},
		},
		"age": &Field{
			Type: Int,
			Resolve: func(rp ResolveParams) interface{} {
				return "age_person"
			},
		},
		"status": &Field{
			Type: String,
			Resolve: func(rp ResolveParams) interface{} {
				return "status_person"
			},
		},
		"sword_present": &Field{
			Type: Boolean,
			Resolve: func(rp ResolveParams) interface{} {
				return "sword_present_person"
			},
		},
		"is_zombie": &Field{
			Type: Boolean,
			Resolve: func(rp ResolveParams) interface{} {
				return "is_zombie_person"
			},
		},
		"survival_rate": &Field{
			Type: Float,
			Resolve: func(rp ResolveParams) interface{} {
				return "survival_rate_person"
			},
		},
	},
}

// User object to denote basic demo object.
var User = GraphQLObject{
	Name: "User",
	Desc: "User object in the system",
	Fields: FieldMap{
		"_uid_": &Field{
			Type:    String,
			Resolve: func(rp ResolveParams) interface{} { return "uid" },
		},
		"_xid_": &Field{
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
