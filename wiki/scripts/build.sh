# HOST=https://wiki.dgraph.io
#HOST=http://localhost:8080
HOST=http://34.208.35.222
VERSION=0.7.3

HUGO_TITLE="Dgraph Doc ${VERSION}" hugo\
     --destination=public/latest\
     --baseURL=$HOST/latest
