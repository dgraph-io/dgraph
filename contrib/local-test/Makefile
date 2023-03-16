DGRAPH_VERSION = local

current_dir = $(shell pwd)

up: ## Start the zero and alpha containers
	DGRAPH_VERSION=$(DGRAPH_VERSION) docker-compose up -d && docker-compose logs --follow alpha zero keepalive

up-with-lambda: ## Start the zero and alpha containers and a lambda container
	DGRAPH_VERSION=$(DGRAPH_VERSION) docker-compose -f docker-compose.yml -f docker-compose-lambda.yml up -d && docker-compose logs --follow alpha zero lambda keepalive

down: ## Stop the containers
	DGRAPH_VERSION=$(DGRAPH_VERSION) docker-compose stop

down-with-lambda: ## Stop the containers (including the lambda container)
	DGRAPH_VERSION=$(DGRAPH_VERSION) docker-compose -f docker-compose.yml -f docker-compose-lambda.yml stop

refresh: ## Kick containers if new dgragh image found
	DGRAPH_VERSION=$(DGRAPH_VERSION) docker-compose up --detach --build

schema-dql: ## Load/update a DQL schema
ifneq (,$(wildcard ./schema.dql))
	curl --data-binary '@./schema.dql' --header 'content-type: application/octet-stream' http://localhost:8080/alter
else
	@echo "No schema.graphql found"
endif

schema-gql: ## Load/update a GraphQL schema
ifneq (,$(wildcard ./schema.graphql))
	curl --data-binary '@./schema.graphql' --header 'content-type: application/octet-stream' http://localhost:8080/admin/schema
else
	@echo "No schema.graphql found"
endif

drop-data: ## Drops all data (but not the schema)
	curl -X POST localhost:8080/alter -d '{"drop_op": "DATA"}'

drop-all: ## Drops data and schema
	curl -X POST localhost:8080/alter -d '{"drop_all": true}'

load-data-gql: ## Loads data from the gql-data.json file
ifneq (,$(wildcard ./gql-data.json))
	docker run -it -v $(current_dir):/export dgraph/dgraph:$(DGRAPH_VERSION) dgraph live -a host.docker.internal:9080 -z host.docker.internal:5080 -f /export/gql-data.json
else
	@echo "No gql-data.json file found"
endif

load-data-dql-json: ## Loads data from the dql-data.json file
ifneq (,$(wildcard ./dql-data.json))
	curl --data-binary '@./dql-data.json' --header 'content-type: application/json' http://localhost:8080/mutate?commitNow=true
else
	@echo "No dql-data.json file found"
endif

load-data-dql-rdf: ## Loads data from the dql-data.rdf file
ifneq (,$(wildcard ./dql-data.rdf))
	curl --data-binary '@./dql-data.rdf' --header 'content-type: application/rdf' http://localhost:8080/mutate?commitNow=true
else
	@echo "No dql-data.rdf file found"
endif

query-dql: ## Runs the query present in query.dql
ifneq (,$(wildcard ./query.dql))
	@curl --data-binary '@./query.dql' -H "Content-Type: application/dql" -X POST localhost:8080/query?qi=true
else
	@echo "No query.dql file found"
endif

query-gql: ## Runs the query present in query.gql and variables in variables.json (requires gql)
ifeq (, $(shell which gql))
	@echo "No gql in $(PATH), download from https://github.com/matthewmcneely/gql/tree/feature/add-query-and-variables-from-file/builds"
else
	@gql file --query-file query.gql --variables-file variables.json --endpoint http://localhost:8080/graphql
endif

help: ## Print target help
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'