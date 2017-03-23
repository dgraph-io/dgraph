# HOST=https://wiki.dgraph.io
HOST=http://localhost:8080
VERSION=0.7.3

HUGO_TITLE="Dgraph Doc ${VERSION}" hugo\
     --destination=public/latest\
     --baseURL=$HOST/latest
