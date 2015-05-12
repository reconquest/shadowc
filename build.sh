#!/bin/bash

set -e -u

if [ $# -eq 0 ]; then
    echo -e "Usage:\n$0 <version>"
    exit 1
fi

VERSION="$1"
PKGDIR="shadowc-deb"
SRCDIR="src"

sed -i 's/\$VERSION\$/'$VERSION'/g' $PKGDIR/DEBIAN/control

mkdir -p $PKGDIR/etc/shadowc
mkdir -p $PKGDIR/usr/bin

git clone https://github.com/reconquest/shadowc $SRCDIR

# dependencies
go get github.com/docopt/docopt-go

(
    cd $SRCDIR
    go build -o shadowc
)

cp -f $SRCDIR/shadowc $PKGDIR/usr/bin/

dpkg -b $PKGDIR shadowc-${VERSION}_amd64.deb

# restore version placeholder
git checkout $PKGDIR
