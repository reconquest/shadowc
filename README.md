# shadowc

**shadowc** it is the client of [the secure login distribution
service](https://github.com/reconquest/shadowd).

## Usage

It's considered that the **shadowd** server is configured earlier and you have
SSL certificate and trusted **shadowd** hosts/addresses. If not,
[see documentation here](https://github.com/reconquest/shadowc).

You should run shadowc during init configuration of server, but actually you
can run shadowc for changing hash entries at anytime when you need change
passwords.

### Options
- `-s <addr>` - set specified login distribution server address. You can specify
    more than one server. All specified addresses should be trusted by SSL
    certificate.
- `-u <user>` - set specified user which needs shadow entry. You can specify
    more than one user.
- `-c <cert>`- set specified certificate file path. (default:
    `/varr/shadowd/cert/cert.pem`)
- `-f <file>` - set specified shadow file path. Can be usable if you use
    `chroot` on your server and shadowc runned outside the `chroot`.

**Warning**

Do not copy `private.key` file to target server with shadowc, copy only
`cert.pem`.

#### Example

Assume, you have certificate file and two shadowd servers on
`shadowd0.in.example.com:8080` and `shadowd1.in.example.com:8080`, certificate
file has been copied to `/data/shadowc/cert.pem` on target server.

So for getting hash entries for engineers John and Smith you should run command
on target server:

```
shadowc -s shadowd0.in.example.com:8888 -s shadowd1.in.example.com:8080 \
    -u john -u smith \
    -c /data/shadowc/cert.pem
```

Afterwards shadowc will overwrite the shadow file (`/etc/shadow`) and change hash entries for
John and Smith.
