#!/bin/bash

set -e -u

PKGDIR="shadowc-deb"
SRCDIR="src"

mkdir -p $PKGDIR/etc/shadowc
mkdir -p $PKGDIR/usr/bin

if [ -d $SRCDIR ]; then
	rm -rf $SRCDIR
fi
git clone ssh://git@git.rn/devops/shadowc.git $SRCDIR

pushd $SRCDIR
count=$(git rev-list HEAD| wc -l)
commit=$(git rev-parse --short HEAD)
VERSION="${count}.$commit"
popd

sed -i 's/\$VERSION\$/'$VERSION'/g' $PKGDIR/DEBIAN/control

# dependencies
export GOPATH=`pwd`
go get github.com/docopt/docopt-go
go get golang.org/x/crypto/ssh
(
    cd $SRCDIR
    go build -o shadowc
)

cp -f $SRCDIR/shadowc $PKGDIR/usr/bin/

dpkg -b $PKGDIR shadowc-${VERSION}_amd64.deb

# restore version placeholder
git checkout $PKGDIR
