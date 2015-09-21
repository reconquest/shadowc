#!/bin/bash

set -e -u

SRCURL="git.rn/devops/shadowc.git"
PKGDIR="shadowc-deb"
SRCROOT="src"
SRCDIR=${SRCROOT}/${SRCURL}

mkdir -p $PKGDIR/etc/shadowc
mkdir -p $PKGDIR/usr/bin
rm -rf $SRCROOT

export GOPATH=`pwd`
go get $SRCURL
pushd $SRCDIR
go build -o shadowc

count=$(git rev-list HEAD| wc -l)
commit=$(git rev-parse --short HEAD)
VERSION="${count}.$commit"
popd

sed -i 's/\$VERSION\$/'$VERSION'/g' $PKGDIR/DEBIAN/control

cp -f bin/shadowc.git $PKGDIR/usr/bin/shadowc

dpkg -b $PKGDIR shadowc-${VERSION}_amd64.deb

# restore version placeholder
git checkout $PKGDIR
