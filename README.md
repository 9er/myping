# myping

Ping multiple hosts at once and display the reachability over time.

![myping screenshot](https://github.com/9er/myping/blob/master/doc/example-screenshot.png)


## Why?!

Sometimes you might want to see the status of your network/serverfarm/etc. live and in a simple but concise graphical presentation. This tool can help monitoring hardware maintenance, rolling out configuration changes, or debug network issues.


## Usage

```
Usage of myping: [OPTIONS] target...
  -c int
    	echo requests per interval (default 3)
  -f string
    	target list file (format: address displayname)
  -i float
    	update interval (default 1)
```

Usage example:

```
./myping example.com example.net example.org example.edu
```

Target list files contain a ping-target and a display-name, separated by a space character. They can also contain empty lines and comments, i.e. lines starting with `#`.

Example target list:
```
# hostname without display name (displayed as "eff.org")
eff.org

# IPv6 literal with display name
2606:4700:4700::1111 APNIC/Cloudflare
2620:fe::9 Quad9
2001:4860:4860::8888 Google DNS

# IPv4 literal with display-name
140.82.121.4 Github

```


## Installation

Needs `go` to build. You can use `make` to fetch dependencies and build the binary. You can optionally run `make setcap` afterwards to set the capability `cap_net_raw`. Without this capability, the program can only send pings if you run it as root (or with `sudo`).

If you are running Arch Linux you can install this package via [AUR](https://aur.archlinux.org/packages/myping/).
