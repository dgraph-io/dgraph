
DIR=$HOME/tmp/icuembed

mkdir -p $DIR

cd $DIR

wget http://download.icu-project.org/files/icu4c/57.1/icu4c-57_1-src.tgz

wget http://download.icu-project.org/files/icu4c/57.1/icu4c-57_1-data.zip

tar xvzf icu4c-57_1-src.tgz
