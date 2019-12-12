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
}

func TestSchema_WithDgraphDirectives(t *testing.T) {
	expectedDgraphSchema := `
	{
	"schema": [{
		"predicate": "Author.country",
		"type": "uid"
	}, {
		"predicate": "Author.dob",
		"type": "datetime",
		"index": true,
		"tokenizer": ["year"]
	}, {
		"predicate": "Author.name",
		"type": "string",
		"index": true,
		"tokenizer": ["hash", "trigram"]
	}, {
		"predicate": "Author.posts",
		"type": "uid",
		"list": true
	}, {
		"predicate": "Author.reputation",
		"type": "float",
		"index": true,
		"tokenizer": ["float"]
	}, {
		"predicate": "Character.appearsIn",
		"type": "string",
		"index": true,
		"tokenizer": ["hash"],
		"list": true
	}, {
		"predicate": "Character.name",
		"type": "string",
		"index": true,
		"tokenizer": ["exact"]
	}, {
		"predicate": "Country.name",
		"type": "string",
		"index": true,
		"tokenizer": ["trigram", "hash"]
	}, {
		"predicate": "Droid.primaryFunction",
		"type": "string"
	}, {
		"predicate": "Employee.ename",
		"type": "string"
	}, {
		"predicate": "Human.starships",
		"type": "uid",
		"list": true
	}, {
		"predicate": "Human.totalCredits",
		"type": "float"
	}, {
		"predicate": "Post.author",
		"type": "uid"
	}, {
		"predicate": "Post.isPublished",
		"type": "bool",
		"index": true,
		"tokenizer": ["bool"]
	}, {
		"predicate": "Post.numLikes",
		"type": "int",
		"index": true,
		"tokenizer": ["int"]
	}, {
		"predicate": "Post.postType",
		"type": "string",
		"index": true,
		"tokenizer": ["hash", "trigram"]
	}, {
		"predicate": "Post.tags",
		"type": "string",
		"index": true,
		"tokenizer": ["exact"],
		"list": true
	}, {
		"predicate": "Post.text",
		"type": "string",
		"index": true,
		"tokenizer": ["fulltext"]
	}, {
		"predicate": "Post.title",
		"type": "string",
		"index": true,
		"tokenizer": ["term", "fulltext"]
	}, {
		"predicate": "Post.topic",
		"type": "string",
		"index": true,
		"tokenizer": ["exact"]
	}, {
		"predicate": "Schema.date",
		"type": "datetime",
		"index": true,
		"tokenizer": ["day"]
	}, {
		"predicate": "Schema.schema",
		"type": "string"
	}, {
		"predicate": "Starship.length",
		"type": "float"
	}, {
		"predicate": "Starship.name",
		"type": "string",
		"index": true,
		"tokenizer": ["term"]
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
		"predicate": "appears_in",
		"type": "string",
		"index": true,
		"tokenizer": ["hash"],
		"list": true
	}, {
		"predicate": "credits",
		"type": "float"
	}, {
		"predicate": "dgraph.author.country",
		"type": "uid"
	}, {
		"predicate": "dgraph.author.dob",
		"type": "datetime",
		"index": true,
		"tokenizer": ["year"]
	}, {
		"predicate": "dgraph.author.name",
		"type": "string",
		"index": true,
		"tokenizer": ["hash", "trigram"]
	}, {
		"predicate": "dgraph.author.posts",
		"type": "uid",
		"list": true
	}, {
		"predicate": "dgraph.author.reputation",
		"type": "float",
		"index": true,
		"tokenizer": ["float"]
	}, {
		"predicate": "dgraph.employee.en.ename",
		"type": "string"
	}, {
		"predicate": "dgraph.topic",
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
		"predicate": "is_published",
		"type": "bool",
		"index": true,
		"tokenizer": ["bool"]
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
	}],
	"types": [{
		"fields": [{
			"name": "Author.name"
		}, {
			"name": "Author.dob"
		}, {
			"name": "Author.reputation"
		}, {
			"name": "Author.country"
		}, {
			"name": "Author.posts"
		}],
		"name": "Author"
	}, {
		"fields": [{
			"name": "Character.name"
		}, {
			"name": "Character.appearsIn"
		}],
		"name": "Character"
	}, {
		"fields": [{
			"name": "Country.name"
		}],
		"name": "Country"
	}, {
		"fields": [{
			"name": "Character.name"
		}, {
			"name": "Character.appearsIn"
		}, {
			"name": "Droid.primaryFunction"
		}],
		"name": "Droid"
	}, {
		"fields": [{
			"name": "Employee.ename"
		}],
		"name": "Employee"
	}, {
		"fields": [{
			"name": "dgraph.employee.en.ename"
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
			"name": "Post.title"
		}, {
			"name": "Post.text"
		}, {
			"name": "Post.tags"
		}, {
			"name": "Post.topic"
		}, {
			"name": "Post.numLikes"
		}, {
			"name": "Post.isPublished"
		}, {
			"name": "Post.postType"
		}, {
			"name": "Post.author"
		}],
		"name": "Post"
	}, {
		"fields": [{
			"name": "Schema.schema"
		}, {
			"name": "Schema.date"
		}],
		"name": "Schema"
	}, {
		"fields": [{
			"name": "Starship.name"
		}, {
			"name": "Starship.length"
		}],
		"name": "Starship"
	}, {
		"fields": [{
			"name": "State.xcode"
		}, {
			"name": "State.name"
		}],
		"name": "State"
	}, {
		"fields": [{
			"name": "dgraph.author.name"
		}, {
			"name": "dgraph.author.dob"
		}, {
			"name": "dgraph.author.reputation"
		}, {
			"name": "dgraph.author.country"
		}, {
			"name": "dgraph.author.posts"
		}],
		"name": "dgraph.author"
	}, {
		"fields": [{
			"name": "dgraph.employee.en.ename"
		}],
		"name": "dgraph.employee.en"
	}, {
		"fields": [{
			"name": "myPost.title"
		}, {
			"name": "text"
		}, {
			"name": "myPost.tags"
		}, {
			"name": "dgraph.topic"
		}, {
			"name": "myPost.numLikes"
		}, {
			"name": "is_published"
		}, {
			"name": "myPost.postType"
		}, {
			"name": "post.author"
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
	}]
	}
	`

	t.Run("graphql schema", func(t *testing.T) {
		common.SchemaTests(t, expectedDgraphSchema)
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
