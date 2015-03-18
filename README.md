# queensland
Named after the cryptid Queensland Tiger.

Queensland is a simple, etcd backed DNS server.  Other such servers
tend to be rather "ambitious" while queensland is designed to be very
simple.

# Features

Queensland supports basic service discovery via DNS using either A or
SRV lookups.

Queensland also provides _helpers_ for node and service announcement.

# Status

Queensland is still under heavy development.

Currently, only IPv4 addresses are supported.

# Installation

`go get github.com/mistifyio/queensland`

Developed and tested with go 1.4

# Usage


Queensland has three modes of operation:

- node - announce the node for basic resolution
- announce - announce a single service
- server - run the DNS server

running `queensland help` will show command line options.


## Node

In _node_ mode, queensland will announce the node. This simply means
that it will put an entry into etcd for the node to be used for base
DNS resolution. Running `queensland node` will use the hostname and
attempt to find the ip address of the node.  On my laptop, for
example, this will insert a record that can be retrieved like:

```
$ etcdctl get /queensland/nodes/helo
{"ip":"192.168.1.56"}
```

The node name and address can be set via the command line:

```
$ queensland help node
Usage:
  queensland node [flags]

 Available Flags:
  -a, --address="": node addresses. will default to first non-localhost IP
  -e, --etcd="http://127.0.0.1:4001": etcd endpoint
  -h, --help=false: help for node
  -i, --interval=300: announce interval
  -n, --name="": node name. will default to hostname.
  -p, --prefix="/queensland": etcd prefix
  -t, --ttl=0: announce ttl.  0 disables
  ```

In general, you would run this as a service on every node.

You could use `etcdctl` or `curl` to set the same data if desired. The
"node mode" is a convenience as it ensures proper namespacing and data
format.

## Announce

_Announce_ mode is used to announce a single instance of a service.  It
is generally ran on the same node as the service is running.  It is
usually ran as a "side-car" service.

Assuming I had a "www" service running on my laptop, I would run
`queensland announce www` and this would insert a record that could be
fetched:

```
$ etcdctl get /queensland/services/www/helo
{"priority":0,"weight":0,"port":0,"target":"helo"}
```

The node name, etc, can be overridden on the command line:
```
$ queensland help announce
 Available Flags:
  -c, --check="": announce check command
  -e, --etcd="http://127.0.0.1:4001": etcd endpoint
  -h, --help=false: help for announce
  -i, --interval=30: announce interval
  -n, --name="": node name. will default to hostname.
  -o, --port=0: announce service port
  -p, --prefix="/queensland": etcd prefix
  -r, --priority=0: announce service priority
  -t, --ttl=60: announce ttl. 0 disables
  -w, --weight=0: announce service weight
  ```

A service announcement can optionally call a check command:
`queensland announce www --check="curl http://127.0.0.1:8888/"`

And it will only put the key when this command returns 0. The check
command is ran using "sh -c".

Currently, the record is not removed when queensland exits or the
check fails - it relies only on the ttl. This behavior is subject to
change and/or will be controlled via command line flags.

You could use `etcdctl` or `curl` to set the same data if desired. The
"announce mode" is a convenience as it ensures proper namespacing and data
format.


## Server

In server mode, queensland runs a simple DNS server. Queensland can serve one and only one domain.  It can serve services
and nodes.

```
$ queensland help sever
Usage:
  queensland server [flags]

 Available Flags:
  -d, --dns-port=15353: dns server port
  -m, --domain=".local": dns domain
  -e, --etcd="http://127.0.0.1:4001": etcd endpoint
  -h, --help=false: help for server
  -o, --http-port=25353: http server port
  -p, --prefix="/queensland": etcd prefix
  -t, --ttl=0: dns ttl
```

Using the above examples, we can lookup both the node and the service:

```
$ dig @127.0.0.1 -p 15353 helo.nodes.local +short
192.168.1.56

$ dig @127.0.0.1 -p 15353 www.services.local +short
192.168.1.56

$ dig @127.0.0.1 -p 15353 www.services.local +short SRV
0 0 0 helo.local.

$ dig @127.0.0.1 -p 15353 www.services.local SRV
;; ANSWER SECTION:
www.services.local.	0	IN	SRV	0 0 0 helo.local.

;; ADDITIONAL SECTION:
helo.local.		0	IN	A	192.168.1.56

```

Notice we can lookup the service using an A or an SRV lookup.


You can also do [RFC 2782](http://en.wikipedia.org/wiki/SRV_record)
style lookups for services:

```
$ dig @127.0.0.1 -p 15353 _www._tcp.services.local SRV
;; ANSWER SECTION:
www.services.local.	0	IN	SRV	0 0 0 helo.local.

;; ADDITIONAL SECTION:
helo.local.		0	IN	A	192.168.1.56

```

This only works for SRV lookups, not A.  Also, the protocol must be
`_udp` or `_tcp` but it does not matter which.


You can also add external services:

```
$ etcdctl set /queensland/services/www/external '{"target": "some.other.domain.","port": 80 }'`
$ dig @127.0.0.1 -p 15353 www.services.local SRV
;; ANSWER SECTION:
www.services.local.	0	IN	SRV	0 0 0 helo.local.
www.services.local.	0	IN	SRV	0 0 80 some.other.domain.

;; ADDITIONAL SECTION:
helo.local.		0	IN	A	192.168.1.56
```


You should run a "real" name server in front of queensland and only
forward the service discover names to it.  You can set a ttl great
than 0 if you want it to be cached.


# Status

While it works, it is not very well tested.

# TODO

- add metrics, etc
- remove records on exit?
- General code clean-up
- template for check command to use Port, name, address, etc?

# Example Usage

An example of a node using systemd.

Run the node announce mode on every node:

```
# /etc/systemd/system/node-announce.service
[Unit]
Description=Queensland Node Announce

[Service]
ExecStart=/usr/sbin/queensland node
User=nobody
RestartSec=30s
Restart=always

[Install]
WantedBy=multi-user.target
```

Suppose we have an app like:

```
# /etc/systemd/system/myapp.service
[Unit]
Description=My App
Wants=myapp-announce.service

[Service]
ExecStart=/opt/myapp/bin/myapp

[Install]
WantedBy=multi-user.target
```

Notice the `Wants=myapp-announce`

And now the service announcement:

```
# /etc/systemd/system/myapp-announce.service
[Unit]
Description=Queensland Announce MyApp
BindsTo=myapp.service

[Service]
ExecStart=/usr/sbin/queensland announce myapp --port 8080 --check "curl http://127.0.0.1:8080/test"
User=nobody
RestartSec=30s
Restart=always
```

Notice the `BindsTo=myapp.service`


