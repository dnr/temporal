
set -x

# ca
#openssl req -x509 -days 30 -newkey ed25519 -keyout ca-key.pem -sha256 -out ca.pem -noenc -subj "/CN=test ca"
#openssl req -x509 -days 30 -newkey ed25519 -keyout badca-key.pem -sha256 -out badca.pem -noenc -subj "/CN=BAD ca"

#### internode cert
####openssl req -x509 -days 30 -newkey ed25519 -CA ca.pem -CAkey ca-key.pem -CAcreateserial -keyout internode-key.pem -sha256 -out internode.pem -noenc -subj "/CN=internode" -addext 'subjectAltName = DNS:internode'

# local certs
#openssl req -x509 -days 30 -newkey ed25519 -CA ca.pem -CAkey ca-key.pem -CAcreateserial -keyout local-key.pem -sha256 -out local.pem -noenc -subj "/CN=local" -addext 'subjectAltName = DNS:local'

# remote cert
openssl req -x509 -days 30 -newkey ed25519 -CA ca.pem -CAkey ca-key.pem -CAcreateserial -keyout remote-key.pem -sha256 -out remote.pem -noenc -subj "/CN=remote" -addext 'subjectAltName = DNS:remote'
#faketime -f '-23.95h' openssl req -x509 -days 1 -newkey ed25519 -CA ca.pem -CAkey ca-key.pem -CAcreateserial -keyout remote-key.pem -sha256 -out remote.pem -noenc -subj "/CN=remote" -addext 'subjectAltName = DNS:remote'  # expires in 5 minutes!!
#openssl req -x509 -days 30 -newkey ed25519 -CA badca.pem -CAkey badca-key.pem -CAcreateserial -keyout remote-key.pem -sha256 -out remote.pem -noenc -subj "/CN=remote" -addext 'subjectAltName = DNS:remote'

# connect:
localflags="--tls-disable-host-verification --address localhost:7233 --tls-ca-path ca.pem --tls-cert-path local.pem --tls-key-path local-key.pem"
remoteflags="--tls-disable-host-verification --address localhost:17233 --tls-ca-path ca.pem --tls-cert-path remote.pem --tls-key-path remote-key.pem"
#temporal operator cluster list $localflags
#temporal operator cluster list $remoteflags
#temporal operator cluster upsert $localflags --frontend-address localhost:17233 --enable-connection
#temporal operator cluster upsert $remoteflags --frontend-address localhost:7233 --enable-connection


exit

"
notes:

I tried to reproduce this in a bunch of ways:

I set up two development servers (A and B) connected to each other with one CA and each one using a different cert.
`refreshInterval` was set to 10s on both.
After setting up the connections, I replaced B's certs with a cert signed by a different CA and then restarted the servers.
As expected, B started logging errors connecting to A (which was rejecting the cert signed by the wrong CA).
(I'm not entirely sure why A didn't log errors connection to B here.)
Without restarting B, I replaced its cert with a good one again.
Eventually (after both worker and history reloaded certs), it stopped logging errors.
I confirmed (by adding printfs) that it does not do another call to grpc.Dial after startup, the cert reload was enough.

Next I tried replacing B's good cert with a bad one again, without reloading.
B continued to use the established connection. I think this is as expected: the certs are used when establishing a new connection.
Then I restarted A. now both of them were logging errors.
I replaced B's cert with a good one, and both eventually stopped logging errors.

Next, I replaced B's cert with one that expires in 5 minutes and started both clusters.
After 5 minutes, I didn't see any errors immediately, but after several more minutes it started reporting errors (client, server):

```
connection error: desc = "transport: authentication handshake failed: tls: failed to verify certificate: x509: certificate has expired or is not yet valid: current time 2023-10-16T13:21:01-07:00 is after 2023-10-16T20:16:47Z"
connection error: desc = "error reading server preface: remote error: tls: expired certificate"
```

I replaced B's cert with a good one again, and both eventually stopped logging errors.

So it looks like cert reloading does seem to work in a local test setup.



unrelated:

I think NewClientBean shouldn't bother trying to eagerly establish all known
connections, since they get 'evicted' by the metadata change callback right
after startup, and then get created on-demand anyway.


"
