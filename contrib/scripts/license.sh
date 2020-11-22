files=$(find . ! -path "./vendor/*" ! -path "./bp128/*" ! -path "./protos/*" -type f -name "*.go")

for f in $files; do
  echo "Processing $f"
  python2 contrib/scripts/license.py $f

  # Start from year.
  year=$(git log --format=%aD $f | tail -1 | awk '{print $4}')
  if [ "$year" != "2018" ]; then
    sed -i "s/Copyright 2018 Dgraph/Copyright $year-2018 Dgraph/g" $f
  fi

  # Format it.
  gofmt -w $f
done
