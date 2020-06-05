/*
 *    Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package directives

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/pkg/errors"
)

func TestRunAll_WithDgraphDirectives(t *testing.T) {
	common.RunAll(t)
	t.Run("dgraph predicate with special characters",
		common.DgraphDirectiveWithSpecialCharacters)
}

func TestSchema_WithDgraphDirectives(t *testing.T) {
	expectedDgraphSchema := `
{
	"schema": [{
		"predicate": "Category.name",
		"type": "string"
	}, {
		"predicate": "Category.posts",
		"type": "uid",
		"list": true
	}, {
		"predicate": "Country.name",
		"type": "string",
		"index": true,
		"tokenizer": ["trigram", "hash"]
	}, {
		"predicate": "Human.starships",
		"type": "uid",
		"list": true
	}, {
		"predicate": "Movie.name",
		"type": "string"
	}, {
		"predicate": "MovieDirector.name",
		"type": "string"
	}, {
		"predicate": "State.capital",
		"type": "string"
	}, {
		"predicate": "State.name",
		"type": "string"
	}, {
		"predicate": "State.xcode",
		"type": "string",
		"index": true,
		"tokenizer": ["trigram", "hash"],
		"upsert": true
	}, {
		"predicate": "User.name",
		"type": "string",
		"index": true,
		"tokenizer": ["hash"],
		"upsert": true
	}, {
		"predicate": "appears_in",
		"type": "string",
		"index": true,
		"tokenizer": ["hash"],
		"list": true
	}, {
		"predicate": "credits",
		"type": "float"
	}, {
		"predicate": "test.dgraph.author.country",
		"type": "uid"
	}, {
		"predicate": "test.dgraph.author.dob",
		"type": "datetime",
		"index": true,
		"tokenizer": ["year"]
	}, {
		"predicate": "test.dgraph.author.name",
		"type": "string",
		"index": true,
		"tokenizer": ["hash", "trigram"]
	}, {
		"predicate": "test.dgraph.author.posts",
		"type": "uid",
		"list": true
	}, {
		"predicate": "test.dgraph.author.reputation",
		"type": "float",
		"index": true,
		"tokenizer": ["float"]
	}, {
		"predicate": "test.dgraph.employee.en.ename",
		"type": "string"
	}, {
		"predicate": "dgraph.graphql.schema",
		"type": "string"
	}, {
		"predicate": "dgraph.graphql.xid",
		"type": "string",
		"index": true,
		"tokenizer": ["exact"],
		"upsert": true
	}, {
		"predicate": "test.dgraph.topic",
		"type": "string",
		"index": true,
		"tokenizer": ["exact"]
	}, {
		"predicate": "dgraph.type",
		"type": "string",
		"index": true,
		"tokenizer": ["exact"],
		"list": true
	}, {
		"predicate": "directed.movies",
		"type": "uid",
		"reverse": true,
		"list": true
	}, {
		"predicate": "hasStates",
		"type": "uid",
		"list": true
	}, {
		"predicate": "inCountry",
		"type": "uid"
	}, {
		"predicate": "is_published",
		"type": "bool",
		"index": true,
		"tokenizer": ["bool"]
	}, {
		"predicate": "myPost.category",
		"type": "uid"
	}, {
		"predicate": "myPost.numLikes",
		"type": "int",
		"index": true,
		"tokenizer": ["int"]
	}, {
		"predicate": "myPost.postType",
		"type": "string",
		"index": true,
		"tokenizer": ["hash", "trigram"]
	}, {
		"predicate": "myPost.tags",
		"type": "string",
		"index": true,
		"tokenizer": ["exact"],
		"list": true
	}, {
		"predicate": "myPost.title",
		"type": "string",
		"index": true,
		"tokenizer": ["term", "fulltext"]
	}, {
		"predicate": "performance.character.name",
		"type": "string",
		"index": true,
		"tokenizer": ["exact"]
	}, {
		"predicate": "post.author",
		"type": "uid"
	}, {
		"predicate": "pwd",
		"type": "password"
	}, {
		"predicate": "roboDroid.primaryFunction",
		"type": "string"
	}, {
		"predicate": "star.ship.length",
		"type": "float"
	}, {
		"predicate": "star.ship.name",
		"type": "string",
		"index": true,
		"tokenizer": ["term"]
	}, {
		"predicate": "text",
		"type": "string",
		"index": true,
		"tokenizer": ["fulltext"]
	}, {
		"predicate": "People.xid",
		"type": "string",
		"index": true,
		"tokenizer": ["hash"],
		"upsert": true
	}, {
		"predicate": "post",
		"type": "string"
	},{
		"predicate": "职业",
		"type": "string"
	}, {
		"predicate": "People.name",
		"type": "string"
	}, {
		"predicate": "Teacher.subject",
		"type": "string"
	}, {
		"predicate": "Teacher.teaches",
		"type": "uid",
		"list": true
	}, {
		"predicate": "Student.taughtBy",
		"type": "uid",
		"list": true
	}],
	"types": [{
		"fields": [{
			"name": "Category.name"
		}, {
			"name": "Category.posts"
		}],
		"name": "Category"
	}, {
		"fields": [{
			"name": "Country.name"
		}, {
			"name": "hasStates"
		}],
		"name": "Country"
	}, {
		"fields": [{
			"name": "test.dgraph.employee.en.ename"
		}, {
			"name": "performance.character.name"
		}, {
			"name": "appears_in"
		}, {
			"name": "Human.starships"
		}, {
			"name": "credits"
		}],
		"name": "Human"
	}, {
		"fields": [{
			"name": "Movie.name"
		}],
		"name": "Movie"
	}, {
		"fields": [{
			"name": "MovieDirector.name"
		}, {
			"name": "directed.movies"
		}],
		"name": "MovieDirector"
	}, {
		"fields": [{
			"name": "State.xcode"
		}, {
			"name": "State.name"
		}, {
			"name": "State.capital"
		}, {
			"name": "inCountry"
		}],
		"name": "State"
	}, {
		"fields": [{
			"name": "User.name"
		}, {
			"name": "pwd"
		}],
		"name": "User"
	}, {
		"fields": [{
			"name": "test.dgraph.author.name"
		}, {
			"name": "test.dgraph.author.dob"
		}, {
			"name": "test.dgraph.author.reputation"
		}, {
			"name": "test.dgraph.author.country"
		}, {
			"name": "test.dgraph.author.posts"
		}],
		"name": "test.dgraph.author"
	}, {
		"fields": [{
			"name": "test.dgraph.employee.en.ename"
		}],
		"name": "test.dgraph.employee.en"
	}, {
		"fields": [{
			"name": "dgraph.graphql.schema"
		}, {
			"name": "dgraph.graphql.xid"
		}],
		"name": "dgraph.graphql"
	}, {
		"fields": [{
			"name": "post"
		}, {
			"name": "职业"
		}],
		"name": "Message"
	}, {
		"fields": [{
			"name": "myPost.title"
		}, {
			"name": "text"
		}, {
			"name": "myPost.tags"
		}, {
			"name": "test.dgraph.topic"
		}, {
			"name": "myPost.numLikes"
		}, {
			"name": "is_published"
		}, {
			"name": "myPost.postType"
		}, {
			"name": "post.author"
		}, {
			"name": "myPost.category"
		}],
		"name": "myPost"
	}, {
		"fields": [{
			"name": "performance.character.name"
		}, {
			"name": "appears_in"
		}],
		"name": "performance.character"
	}, {
		"fields": [{
			"name": "performance.character.name"
		}, {
			"name": "appears_in"
		}, {
			"name": "roboDroid.primaryFunction"
		}],
		"name": "roboDroid"
	}, {
		"fields": [{
			"name": "star.ship.name"
		}, {
			"name": "star.ship.length"
		}],
		"name": "star.ship"
	}, {
		"fields": [{
			"name": "People.xid"
		}, {
			"name": "People.name"
		}],
		"name": "People"
	}, {
		"fields": [{
			"name": "People.xid"
		}, {
			"name": "People.name"
		}, {
			"name": "Teacher.subject"
		}, {
			"name": "Teacher.teaches"
		}],
		"name": "Teacher"
	}, {
		"fields": [{
			"name": "People.xid"
		}, {
			"name": "People.name"
		}, {
			"name": "Student.taughtBy"
		}],
		"name": "Student"
	}]
}
	`

	t.Run("graphql schema", func(t *testing.T) {
		common.SchemaTest(t, expectedDgraphSchema)
	})
}

func TestMain(m *testing.M) {
	schemaFile := "schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		panic(err)
	}

	jsonFile := "test_data.json"
	data, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		panic(errors.Wrapf(err, "Unable to read file %s.", jsonFile))
	}

	common.BootstrapServer(schema, data)

	os.Exit(m.Run())
}
