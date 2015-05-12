# shadowc

**shadowc** is a client for
[the secure login distribution service](https://github.com/reconquest/shadowd).

**shadowc** is not a user management tool, shadowc can not create
user/group, shadowc is hash entries manager, which communicate with **shadowd**
server through REST API and request hash entries for users, afterwards
**shadowc** overwrite `/etc/shadow` file with new hash entries.

## Usage

It's considered that the **shadowd** server is configured earlier and you have
SSL certificate and trusted **shadowd** hosts. If not,
[see documentation here](https://github.com/reconquest/shadowd).

**shadowc** can be used either on initial server configuration or for changing
hash entries anytime when you need change passwords.

### Options
- `-s <addr>` — use specified login distribution server address. You can specify
    more than one server. All specified addresses should be trusted by SSL
    certificate.
- `-u <user>` — set specified user which needs shadow entry. You can specify
    more than one user.
- `-p <pool>` — use specified hash tables pool. (default: `main`)
- `-c <cert>` — set specified certificate file path. (default:
    `/var/shadowd/cert/cert.pem`)
- `-f <file>` — set specified shadow file path. Can be usable if you use
    `chroot` on your server and shadowc runned outside the `chroot`. (default:
    `/etc/shadow`)

**Warning**

Do not copy `key.pem` file to target server with shadowc, copy only
`cert.pem`.

#### Example

Assume that, you have certificate file and two shadowd servers on
`shadowd0.in.example.com:8080` and `shadowd1.in.example.com:8080`, certificate
file has been copied to `/data/shadowc/cert.pem` on target server.

If you want to grant password access for users `john` and `smith` on some
server, you should invoke following command, which will obtain unique hashes
for that server for specified users and place them into `/etc/shadow` file.

After that, `john` and `smith` will be able to connect to the server using
passwords, which were set on **shadowd** server while generating hash tables
for that users.

```
shadowc -s shadowd0.in.example.com:8888 -s shadowd1.in.example.com:8080 \
    -u john -u smith \
    -c /data/shadowc/cert.pem
```

Afterwards shadowc will overwrite the shadow file (`/etc/shadow`) and change
hash entries for John and Smith.
