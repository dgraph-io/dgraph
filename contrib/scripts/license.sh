files=$(find . ! -path "./vendor/*" ! -path "./bp128/*" -type f -name "*.go")

cat > /tmp/notice << EOF
/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

EOF

echo "NOTICE IS:"
cat /tmp/notice

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

