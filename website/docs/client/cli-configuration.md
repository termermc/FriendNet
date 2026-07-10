# CLI Configuration
```
./friendnet -help

Usage of ./friendnet:
  -datadir string
    	path to the client's data directory
  -davaddr string
    	WebDAV server address (default "https://127.0.0.1:20043")
  -headless
    	run client in headless mode (RPC-only, no web UI, no locking, no GUI or browser functionality)
  -installca
    	if set, tries to install the client's root CA for HTTPS on the web UI
  -nobrowser
    	do not open web UI in browser
  -nolock
    	do not use a lock to prevent multiple instances of the client from running
  -pproffile string
    	write CPU profile data in the pprof format to this file, e.g. "cpu.pprof"
  -resettoken
    	if set, resets the bearer token for the RPC server
  -rmcerthost string
    	removes the specified host from the certificate store (like removing a host from SSH known_hosts)
  -uninstallca
    	if set, tries to uninstall the client's root CA
  -webaddr string
    	web UI and RPC address (default "https://127.0.0.1:20042")
```
## Set WebUI Connection
Configuring the port and IP the WebUI listens on is done via `-webaddr`.

### Examples
Use port `6000`:
```
./friendnet -webaddr "https://127.0.0.1:6000"
```

Make the WebUI available on all interfaces (reachable from other machines):
```
./friendnet -webaddr "https://0.0.0.0:20042"
```
> NOTE: Ports 1 to 1023 are considered privileged under Linux and require root or additional configuration which is out of scope for this documentation.

## Set WebDAV Connection
Same as modifying the WebUI but using `-davaddr`
