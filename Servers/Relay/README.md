# Servers relay

The relay is the server that Servers mode matches players through. This file covers the VPN check. The protocol is in `docs/servers/PROTOCOL.md`, sections 4.4 and 7.2.

## Build

The build file is `CMakeLists.txt` in this folder. It is standalone and does not use the game build. It needs Visual Studio, CMake and Ninja, and network access on the first configure, because ENet, libsodium and libmaxminddb are fetched and pinned there. libmaxminddb is 1.14.1, fetched by commit.

```
cmake -S Servers/Relay -B <build dir> -G Ninja -DCMAKE_BUILD_TYPE=Release
cmake --build <build dir>
```

## VPN check

With `vpn_check = on`, the relay refuses an entry check from an address that is in a listed network range or belongs to a listed network operator. Refused players get `REFUSE VPN_DETECTED` before any ping is measured. The allow list overrides both lists.

The relay reads every data file into memory at start. A lookup does not touch a file or the network.

### Data files

Each file is named in `relay.conf`. Blank lines and lines starting with `#` are skipped. Lines may end in LF or CRLF. A line longer than 512 bytes is an error.

| Key | File format | Example line |
|---|---|---|
| `vpn_ranges` | One IPv4 network per line, refused. | `198.51.100.0/24` |
| `vpn_operators` | One network operator number per line, refused. Decimal, 0 to 4294967295. | `64496` |
| `vpn_operator_db` | A MaxMind DB file (`.mmdb`) that maps an address to its network operator. | (binary) |
| `vpn_allow` | One IPv4 network per line, always allowed. | `203.0.113.7` |

A network is written as `a.b.c.d/n`, with `n` from 0 to 32. A bare address is one host, the same as `/32`. Host bits are ignored, so `198.51.100.77/24` covers `198.51.100.0` to `198.51.100.255`. IPv6 is not supported.

The operator number is the autonomous system number (ASN) in the database's `autonomous_system_number` field.

### Turning each check off

The three checks are switched by their files. An empty list file switches that list off. `vpn_check = off` switches all of them off, and then no file is read, so missing files are not an error.

### Where to get the data

- **Operator database.** MaxMind's GeoLite2 ASN database is free. Its download needs a free MaxMind account and a licence key, and it is updated regularly. Download `GeoLite2-ASN.mmdb` and point `vpn_operator_db` at it. Follow MaxMind's licence terms. Do not commit the file.
- **Range and operator lists.** These files are not supplied with the relay. The project owner chooses the source. Check the licence of any source before use.

The `data/` folder named in `relay.conf` is not in the repository.

### Start and reload

- At start, every file must load. A missing or invalid file stops the relay with a message that names the file, unless the check is off.
- In the relay console, press Ctrl+Break to reload the data without a restart. A reload that fails logs the reason and keeps the data already in use.

### Logs

A refusal is logged with its reason and a shortened address. The last octet is hidden:

```
connection 7: VPN check refused 203.0.113.x, listed network range
connection 8: VPN check refused 203.0.113.x, listed network operator 64496
```

The relay never logs a full address at normal levels.

## Tests

`relay_tests` covers the VPN check. The test writes its own small MaxMind DB, so it needs no downloaded file. Build with `-DRELAY_BUILD_TESTS=ON` and run `ctest --test-dir <build dir>`.
