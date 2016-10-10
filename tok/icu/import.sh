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

rm -Rf $TARGET/icuembed
mkdir -p $TARGET/icuembed

cp -f icu/source/common/*.cpp $TARGET/icuembed
cp -f icu/source/common/*.c $TARGET/icuembed
cp -f icu/source/common/*.h $TARGET/icuembed
cp -Rf icu/source/common/unicode $TARGET/icuembed

#cp -Rf icu/source/common $TARGET
cp -f icu/source/stubdata/stubdata.c $TARGET/icuembed

cp -f $DIR/$DATA $TARGET

# Add in some Go file(s).
cd $TARGET
cp -f icuembed.go.tmpl icuembed/icuembed.go

echo -e "Please run dgraph with the flag:\n-icu $TARGET/$DATA"