# ANCHOR

*anchor* manages dns records for docker containers.

inspired by [skydock](https://github.com/crosbymichael/skydock).

## requirements

* [docker](https://github.com/docker/docker)
* [etcd](https://github.com/coreos/etcd)
* [skydns](https://github.com/skynetservices/skydns)

## not required but nice to have

* [systemd](https://github.com/systemd/systemd)

## example

## running

### [CoreOS](https://coreos.com)
*anchor* is pretty useful on coreos with flannel.

first get skydns running. an example systemd unit is at [skydns.service](systemd/coreos/skydns.service).

to use anchor on a coreos cluster, download [anchor.service](systemd/coreos/anchor.service)
and run

	fleetctl start anchor.service

you should also enable use of the dns server in systemd-resolved, so edit
`/etc/systemd/resolved.conf` and add the LAN ip of the coreos host to the `DNS=` key.

for example, if 10.0.0.10 is the LAN ip of the CoreOS host:

```
[Resolve]
DNS=10.0.0.10
```

### dev environment
*anchor* is handy to run in your dev environment too.

it is assumed you have systemd and that your user is in the docker group.

place [etcd2.service](systemd/user/etcd2.service), [skydns.service](systemd/user/skydns.service), [anchor.service](systemd/usr/anchor.service) in
`~/.config/systemd/user/` and run

	systemctl --user daemon-reload
	systemctl --user start anchor.service

then add 127.0.0.1 to your list of dns servers, e.g. in `/etc/resolv.conf.head`.

### using it

run a few servers and give them a moment to register in skydns:

	docker run --name apache1 -d coreos/apache /usr/sbin/apache2ctl -D FOREGROUND
	docker run --name apache2 -d coreos/apache /usr/sbin/apache2ctl -D FOREGROUND
	docker run --name apache3 -d coreos/apache /usr/sbin/apache2ctl -D FOREGROUND

now resolve their addresses:

	dig @127.0.0.1 apache.dev.skydns.local

and you will see something like...

	;; ANSWER SECTION:
	apache.dev.skydns.local. 8      IN      A       172.17.0.20
	apache.dev.skydns.local. 8      IN      A       172.17.0.19
	apache.dev.skydns.local. 8      IN      A       172.17.0.21

hooray!

if you are on CoreOS, you can try `curl -v http://apache.dev.skydns.local/` to test apache httpd.
in a dev environment, visit http://apache.dev.skydns.local/ in your browser.

cleanup the docker containers:

	docker rm -f apache1 apache2 apache3

notice that they have been unregistered by *anchor*:

	$ dig @127.0.0.1 apache.dev.skydns.local +short
	$ 

