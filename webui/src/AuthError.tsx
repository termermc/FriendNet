import type { Component } from 'solid-js'

const AuthError: Component = () => {
	return (
		<div class="auth-error">
			<div class="auth-card">
				<h1>Authorization required</h1>
				<p>
					No bearer token was found in the URL or local storage. The FriendNet
					 web UI needs a bearer token to access the local client RPC service.
				</p>
				<p>Launch the UI with a token in the query string, for example:</p>
				<code class="code-block">
					?bearerToken=YOUR_TOKEN_HERE
				</code>
				<p>
					Once it is present, the token is stored in local storage and reused on
					 future visits.
				</p>
			</div>
		</div>
	)
}

export default AuthError
