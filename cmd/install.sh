set -e

for d in dgraph dgraphassigner dgraphlist dgraphloader dgraphmerge; do
  echo "Install $d"
  cd $d && go build -a . && go install . && cd ..
done
