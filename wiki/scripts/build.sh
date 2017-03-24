# HOST=https://wiki.dgraph.io
HOST=http://localhost:8080

hugo --destination=public/latest \
       --baseURL=$HOST/latest
