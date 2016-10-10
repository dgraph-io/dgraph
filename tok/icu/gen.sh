TARGET=`pwd`
DIR=/tmp/icu
DATA=icudt57l.dat

mkdir -p $DIR
cd $DIR

if [ ! -f $DIR/icu4c-57_1-src.tgz ]; then
  wget http://download.icu-project.org/files/icu4c/57.1/icu4c-57_1-src.tgz
	tar xvzf icu4c-57_1-src.tgz
fi

if [ ! -f $DIR/$DATA ]; then
  wget https://github.com/dgraph-io/experiments/raw/master/icu/$DATA
fi

# Add: Get data from our github page.

rm -Rf $TARGET/common
rm -Rf $TARGET/icuembed

cp -Rf icu/source/common $TARGET
cp -f icu/source/stubdata/stubdata.c $TARGET/common
mv $TARGET/common $TARGET/icuembed

cp $DIR/$DATA $TARGET

# Add in some Go files.
cd $TARGET
cp -f icuembed.go.tmpl icuembed/icuembed.go
