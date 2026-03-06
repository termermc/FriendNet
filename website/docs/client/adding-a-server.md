# Adding a Server

On its own, a FriendNet client can't do anything; it needs a server to find other users
and to share files with them. Your client can connect to as many server as you'd like at
once, and share different folders for each.

> Want to create your own server? Check out the [setup guide](../server/setup/index_setup.md).

To add a server, click the ➕️ icon on the `Servers` label:

![add server icon](add-server-icon.png)

From there, you will be presented with a form:

![add server form](add-server-form.png)

In the `Name` field, you can put whatever you want; it is the label shown only in your client.

The `Address` field is how to reach the server. It can be a domain name with a port, like
`example.com:20038`, a domain name without a port, like `example.com` (if the server's port
is the default, `20038`), or a bare IP address with or without port, depending on whether the
server is using the default port.

The `Room` field is the name of the [room](../rooms.md) to join.

The `Username` and `Password` fields are the credentials for the room account you are signing
in with.

Once you have filled in all the fields, click the `Add Server` field to add it.

You should see the server you just added in the server browser panel on the left:

![server](server.png)

If you made any mistakes, you can click the `📝️` icon on the server to edit it.

If you see that the server's icon is red, then the server isn't connected. To figure out why
the server is not connecting, you can click on the `🔎 Log Viewer` on the top of the client
to see recent client log messages. Search for messages like `Failed to create room connection`
and read the details under them. You may have entered the address wrong or given incorrect
credentials. You can manually reconnect by clicking `Connect` under the server once you have
corrected the mistakes.

Now that you have added your server, you can browse shares from other online users, and share
your own folders.

To add a share, click `📁 Manage Shares` on the server:

![manage shares](manage-shares.png)

The `Name` field is the name to give to the share. This is the name other users will see when
browsing your shares.

The `Local Path` field is where on your computer the folder is located. It must be the absolute
path to the folder, like `C:\Users\YourUsername\Music` (on Windows),
`/Users/YourUsername/Music` (on Mac), or `/home/YourUsername/Music` (on Linux).

The `Follow symbolic links?` field determines whether the share will allow symbolic links. If you
do not know what that is, keeping it checked is the best option. If unchecked, symbolic links
will be excluded and treated as if they do not exist. This is the safest option if you know you
have symbolic links that could lead to folders you do not want shared.

Once you have added the share, any user will be able to able to browse it until you remove it.
These shares only apply to this server; you will need to configure shares separately for other
servers you are connected to.

To confirm that you have added the share, click `📂 Browse` on yourself under the server.

![self shares](self-shares.png)

Congratulations! You have now shared your first folder.

Next: [Profiles](profiles.md)
