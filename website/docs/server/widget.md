# Widget

If you want to show the status of your server and online room users on a webpage, you can use the
`friendnet-server-widget` web component on your website.

To include the widget code, add the following HTML to your page:

```html
<script type="module" async defer src="https://cdn.jsdelivr.net/npm/friendnet-server-widget@latest/dist/friendnet-server-widget.js"></script>
```

If you use NPM, you can install it with:

```shell
npm install friendnet-server-widget
```

Then, include the component in your HTML:

```html
<friendnet-server-widget
	rpc="http://localhost:8080"
	room="roomname"
	label="My Friend Group Server (can be empty)"
	token="set token if needed"
/>
```

Replace `http://localhost:8080` with the URL of your FriendNet's public RPC interface.

Your RPC interface must support at least the following methods:

- `GetRoomInfo`
- `GetOnlineUsers`

Remember that publicly exposing other methods, especially action/update methods, is dangerous.

You should now have a widget for your server!

> ![screenshot](widget.png)
