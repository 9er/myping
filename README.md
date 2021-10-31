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

Example:

```
myping example.com example.net example.org example.edu
```


## Installation

Needs `go` to build. You can use `make` do fetch dependencies and build the binary. You can optionally run `make setcap` afterwards to set the capability `cap_net_raw`. Without this capability, the program can only send pings if you run it as root (or with `sudo`).
