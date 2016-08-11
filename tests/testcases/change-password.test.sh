:shadowd

:shadowd-set-response <<OUT
200

\$5\$abcdef
\$5\$123456
OUT

oldpassword="old-password"
password="new-password"
proofpassword="new-password"

tests:ensure expect <<EXPECT
  set timeout -1
  spawn shadowc.test --trace -c tls.crt -P -s $_shadowd -p ops -u operator
  expect {
    Password: {
        send "$password\r"
        exp_continue
    } "New password:" {
        send "$password\r"
        exp_continue
    } "Repeat new password:" {
        send "$password\r"
        exp_continue
    } eof {
        send_error "\$expect_out(buffer)"
        exit 0
    }
  }
EXPECT

shadow1="%245%24abcdef%24ly9hxeVhNT8%2FFKppVb3faXkQd8lx%2Fo%2F96JZtM2p5UJ0"
shadow2="%245%24123456%24Z9PaTbPVdd3jgx9IhGUCvdXiKSAz1HK0JMr.sQqKnAB"

tests:assert-no-diff shadowd_request/body/raw <<BODY
password=new-password&shadow%5B%5D=$shadow1&shadow%5B%5D=$shadow2
BODY
