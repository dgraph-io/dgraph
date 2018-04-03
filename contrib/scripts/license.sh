files=$(find . ! -path "./vendor/*" -type f -name "*.go")

for f in $files; do
  year=$(git log --format=%aD $f | tail -1 | awk '{print $4}')
  echo $f, $year
  sed -i.bak "s/Copyright 2018/Copyright $year-2018/g" $f
done

