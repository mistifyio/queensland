# queensland
Named after the cryptid Queensland Tiger.

Queensland is a simple, etcd backed DNS server.  Other such servers
tend to be rather "ambitious" while queensland is designed to be very
simple.

# Features

Queensland supports basic service discovery via DNS using either A or
SRV lookups.

# Installation

`go get github.com/mistifyio/queensland`

Developed and tested with go 1.4

# Usage

```
Usage of queensland:
  -address=":15353": UDP address to listen
  -domain="local.": domain - must end with '.'
  -etcd="http://localhost:4001": etcd address
  -prefix="/": etcd prefix
  -ttl=0: DNS TTL for responses
  ```

Queensland can serve one and only one domain.  It can serve services
and nodes.

To add a node:

`$ etcdctl set /nodes/bar '{"ip": "10.10.1.3"}'`

And to look it up:

```
$ dig @127.0.0.1 -p 15353 bar.nodes.local +short
10.10.1.3
```

To add a service, we just add a node:

`$ etcdctl set /services/www/bar '{"target": "bar"}'`

Note: the key we add ("bar" in this example) should be unique for this
service.  A good practice is to use the node name.

Now, a loop up for the service:

```
$ dig @127.0.0.1 -p 15353 www.services.local SRV
...
;; ANSWER SECTION:
www.services.local.	0	IN	SRV	0 0 0 bar.local.

;; ADDITIONAL SECTION:
bar.local.		0	IN	A	10.10.1.3
...
```

You can also add external services:

`$ etcdctl set /services/www/external '{"target": "some.other.domain.","port": 80 }'`
`dig @127.0.0.1 -p 15353 www.services.local SRV`

```
...
;; ANSWER SECTION:
www.services.local.	0	IN	SRV	0 0 0 bar.local.
www.services.local.	0	IN	SRV	0 0 80 some.other.domain.

;; ADDITIONAL SECTION:
bar.local.		0	IN	A	10.10.1.3
...
```

Note: for nodes, you generally should add them with a ttl and "touch"
them periodically via an external mechanism.


Queensland also allows legacy services to use it for discovery just
doing A record lookups.

`dig @127.0.0.1 -p 15353 www.services.local`
```
...
;; ANSWER SECTION:
www.services.local.	0	IN	A	10.10.1.3
...
```

You should run a "real" name server in front of queensland and only
forward the service discover names to it.  You can set a ttl great
than 0 if you want it to be cached.


# Status

While it works, it is not very well tested.

# TODO

- support  RFC 2782 style lookups using \_service.\_proto.domain.

- add metrics, etc
