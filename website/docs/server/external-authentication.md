# External Authentication & Middleware

By default, the FriendNet server uses its own account system to authenticate clients to rooms. However, it is also
possible to integrate an external authentication systems and middleware.

External authentication can be useful if you want to integrate FriendNet with an existing community, such as a forum,
IRC, etc.

You can also configure middleware to block authentication attempts based on IP or credentials, but pass them to the next
provider (or the built-in account system if there are no other providers) if the middleware approves.

## Configuration

To configure external authentication, add the `external_auth` field to your `server.json`:

```json
{
    "external_auth": {
        "global": [
            {
                "command": {
                    "name": "/usr/local/bin/ip_blocker.py"
                }
            },
            {
                "command": {
                    "name": "/usr/local/bin/country_blocker.py"
                }
            }
        ],
        "rooms": {
            "forum": {
                "providers": [
                    {
                        "http": {
                            "url": "https://forum.example.com/api/friendnet_auth.php"
                        }
                    }
                ]
            }
        }
    }
}
```

The `external_auth` object contains two possible (but optional) fields: `global` and `rooms`.

`global` is a list of providers that apply to every room. By default, they run before any room-specific providers.

`rooms` is a mapping of room names to provider settings. The required field for `rooms` is `providers`, which is a list
of providers that apply only to that room. You can also set `before_global` to `true`, which will make the room-specific
providers run before the `global` providers.

## Providers

External authentication is implemented by a *provider*: a script or an HTTP endpoint.

A provider reads a JSON authentication request that looks like this:

```json
{
    "type": "auth",
    "ip": "127.0.0.1",
    "room": "filesharing",
    "username": "john_smith",
    "password": "plsdonotsteal"
}
```

...and writes a response that looks like this:

```json
{
    "status": "ok"
}
```

...or this:

```json
{
    "status": "bad",
    "reason": "we just don't like you"
}
```

If `status` is `ok`, the client will be let in. If it's `bad`, the client's authentication attempt will be rejected,
along with a message set by `reason` (or a generic message if `reason` is omitted).

If you would like to generate types or do validation on requests or responses, there are JSON schemas defined for them:
 - [external-auth-request.schema.json](/schema/external-auth-request.schema.json)
 - [external-auth-response.schema.json](/schema/external-auth-response.schema.json)

Every provider's JSON can contain an optional `timeout_seconds` field, which dictates how many seconds to wait on the
provider before giving up and returning an error.

Currently, two different provider types are supported: HTTP and command.

### Command Providers

Command providers let you implement authentication using a script or a shell command. Whenever a client attempts to
authenticate, the server will execute the command/script, writing the request to stdin and reading the response from
stdout. As long as the script returns a valid response and a non-error status code, it is valid.

A command provider's JSON looks like this:

```json
{
    "timeout_seconds": 10,
    "command": {
        "name": "/path/to/script",
        "args": ["arg1", "arg2"],
        "environment": {
            "SECRET": "VALUE"
        }
    }
}
```

The `name` field is required, and is the path or name of the script/command. If it is a relative path like
`./script.sh`, it will be relative to the server's current working directly. If it is a bare command like `validate` and
not a path, it will be resolved based on the system's `PATH`.

The `args` field is an optional list of arguments to pass to the script.

The `environment` field is a key-value object of environment variables to pass to the script. This can be useful for
passing an authorization token to the script. Note that scripts do not inherit the server's environment variables,
except for `PATH`.

Scripts can be written in any language, as long as they are executable by the system shell.

Here's an example script written in Python that allows any user whose password matches their username:

```python
#!/usr/bin/python3

import sys, json

raw_input = sys.stdin.readline()
input = json.loads(raw_input)

reqType = input.get('type', '')
if reqType != 'auth':
    print(json.dumps({'status': 'bad', 'reason': 'unknown request type'}))
    sys.exit(0)

username = input.get('username', '')
password = input.get('username', '')

if username != password:
    print(json.dumps({'status': 'bad' }))
    sys.exit(0)

print(json.dumps({'status': 'ok'}))
```

### HTTP Providers

Command providers let you implement authentication using an HTTP endpoint. Whenever a client attempts to authenticate,
the server will send a POST request to the provider URL, writing the request as a JSON body and reading the response
from HTTP response.

Endpoints must accept a POST request with a `Content-Type: application/json` JSON body, and reply with a 200 status
code, `Content-Type: application/json`, and a valid JSON response.

An HTTP provider's JSON looks like this:

```json
{
    "timeout_seconds": 10,
    "http": {
        "url": "https://forum.example.com/api/friendnet_auth.php",
        "headers": {
            "Authorization": "Bearer abc"
        }
    }
}
```

The `url` field is required, and is the URL to make the POST request to. It can be `http://`, `https://` or `unix://`.
If `unix://`, it must be a path to an HTTP server listening on a UNIX socket. For example, `unix:///var/run/auth.sock`.
If the UNIX socket is a relative path, it will be relative to the server's current working directory.

The `headers` field is an optional key-value object of header names and values. By default, the only header sent by the
server is `Content-Type: application/json`, but additional headers can added with this field. This can be useful for
authorization.

Here's an example endpoint written in PHP that allows any user whose password matches their username and requires
a valid authorization header:

```php
<?php

header('Content-type: application/json');

if (($_SERVER["HTTP_AUTHORIZATION"] ?? '') !== 'Bearer abc') {
    echo json_encode(['status' => 'bad', 'reason' => 'invalid authorization header']);
    die();
}

$body = file_get_contents('php://input');
$req = json_decode($body, true);

$room = $req['room'];
$username = $req['username'];
$password = $req['password'];

if ($username !== $password) {
    echo json_encode(['status' => 'bad']);
    die();
}

echo json_encode(['status' => 'ok']);
```

## Middleware

You may have noticed in the config examples that you can provide multiple providers in an array. This is because 
providers can act as middleware. Instead of returning `{"status":"ok"}`, a provider can return `{"status":"pass"}` to
indicate that it approves of the request and to send it to the next provider. If there are no other providers to check
with, it will fall back to the server's built-in account system.

Middleware providers are useful for IP filtering, rate limiting, etc. You could use them to block IPs from certain
countries, throttle connection attempts, or even prevent clients from connecting on certain days of the week.

Middleware providers are still normal command or HTTP providers; they only become a middleware provider the moment they
return `pass`. This means that providers can conditionally act as middleware.
