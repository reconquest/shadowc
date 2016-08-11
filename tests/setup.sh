tests:clone ../shadowc bin/shadowc.test
tests:clone shadowd.mock bin/

tests:ensure chmod +x bin/shadowd.mock

_shadowd="localhost:64777"

:shadowd() {
    tests:value _blankd \
        $(which blankd) \
        -l "$_shadowd" \
        --tls \
        -o $(tests:get-tmp-dir)/blankd.log \
        -d $(tests:get-tmp-dir)/ \
        -e $(tests:get-tmp-dir)/bin/shadowd.mock
    tests:put-string _blankd_process "$_blankd"
}

:shadowd-set-response() {
    tests:put shadowd_response${1:+_$1}
}
