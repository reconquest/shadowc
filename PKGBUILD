pkgname=shadowc
pkgver=36.7c08402
pkgrel=1
pkgdesc="client for shadowd"
url="https://github.com/reconquest/shadowc"
arch=('i686' 'x86_64')
license=('GPL')
makedepends=('go')

source=("git://github.com/reconquest/shadowc.git")
md5sums=('SKIP')
backup=()

pkgver() {
    cd "${pkgname}"
    echo $(git rev-list --count master).$(git rev-parse --short master)
}

build() {
    cd "$srcdir/$pkgname"

    rm -rf "$srcdir/.go/src"

    mkdir -p "$srcdir/.go/src"

    export GOPATH=$srcdir/.go

    mv "$srcdir/$pkgname" "$srcdir/.go/src/"

    cd "$srcdir/.go/src/shadowc/"
    ln -sf "$srcdir/.go/src/shadowc/" "$srcdir/$pkgname"

    go get
}

package() {
    mkdir -p "$pkgdir/usr/bin"
    mkdir -p "$pkgdir/etc/shadowc/"
    cp "$srcdir/.go/bin/$pkgname" "$pkgdir/usr/bin"
}
