if [[ -f _blankd_process ]]; then
    tests:eval kill -9 "$(cat _blankd_process)"
fi

if [[ -f $(tests:get-tmp-dir)/blankd.log ]]; then
    cat $(tests:get-tmp-dir)/blankd.log
fi
