# cnr-rrpgwd

`rrpgwd` is a Go daemon that fronts an upstream XRRP server (CentralNic Reseller /
RRPproxy). It accepts PTF-format requests over plain TCP, multiplexes them onto a
pool of authenticated TLS sessions to the upstream, and returns the PTF response.

## Build

```sh
go build -o bin/rrpgwd   ./cmd/rrpgwd.go     # the daemon
go build -o bin/rrpcli    ./cmd/rrpcli.go      # the PTF client
go build -o bin/ptfdemo ./cmd/ptfdemo.go   # Describe demo (talks to a running rrpgwd)
```

(`go build ./...` won't work — `cmd/` holds separate `package main` entrypoints.)

## Configure & run

Configuration is layered: built-in defaults < a YAML file < `RRPGWD_*` environment
variables. Copy the example and provide upstream credentials:

```sh
cp rrpgwd.example.yaml rrpgwd.yaml
# edit rrpgwd.yaml, or:
export RRPGWD_USERNAME=... RRPGWD_PASSWORD=...
./bin/rrpgwd
```

The config file path defaults to `./rrpgwd.yaml` and can be overridden with
`$RRPGWD_CONFIG`; a missing file is not an error. See `rrpgwd.example.yaml` for all keys.

## `rrpcli` client

`rrpcli` sends a PTF command to a running `rrpgwd` and prints the response. It
builds the request from command-line arguments:

```sh
# command + parameters (param keys are upper-cased)
rrpcli StatusDomain domain=example.com

# indexed list parameter: key,=a,b  ->  KEY0=a, KEY1=b
rrpcli AddDomain domain=example.com nameserver,=ns1.example.com,ns2.example.com

# pipe raw PTF on stdin when no command word is given
printf '[COMMAND]\ncommand=Describe\ntarget=protocol\nEOF\n' | rrpcli
```

Options:

| Flag | Effect |
|------|--------|
| `-v` | print `<code> <description>` to stderr |
| `-p[NAME]` | print property `NAME`'s values, or every property as `KEY:value` if bare |
| `-OK<code>` | treat `<code>` as success (exit 0); defaults to 200 |
| `--socket=URL` | daemon socket `rrpgwd://user:pass@host:port` |

The socket defaults from `--socket=`, then `$RRPCLI_SOCKET`, then
`rrpgwd://test:test@127.0.0.1:2000`. The default output is the raw PTF response,
and the process exit code is the response code (0 when it matches the ok-code).

## Test

```sh
go test ./internal/... ./pkg/...
```

(`./...` won't work — the two `cmd/` entrypoints share `package main`.)
