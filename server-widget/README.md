# Server Widget

This is a tiny widget that can be embedded on webpages to display info about a FriendNet server.

Run `npm run build` to build the library, then embed the output JS in your webpage and add the web component:

```html
<friendnet-server-widget
	rpc="http://localhost:8080"
	room="roomname"
	label="My Friend Group Server"
	token="set token if needed"
/>
```

Replace `http://localhost:8080` with the URL of your FriendNet's public RPC interface.

Your RPC interface must support at least the following methods:

- `GetRoomInfo`
- `GetOnlineUsers`
