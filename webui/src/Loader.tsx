import {
	Component,
	createSignal,
	ErrorBoundary,
	Show,
	Suspense,
} from 'solid-js'
import { bearerTokenKey, FileServerUrlCtx, RpcClientCtx, rpcUrlKey } from './ctx'
import App from './App'
import { createClient, Interceptor } from '@connectrpc/connect'
import { ClientRpcService } from '../pb/clientrpc/v1/rpc_pb'
import { createConnectTransport } from '@connectrpc/connect-web'
import { createAsync } from '@solidjs/router'

const NoRpc: Component = () => {
	return (
		<div>
			<h1>Missing or Invalid RPC URL</h1>
			<p>
				The URL you opened should have contained an RPC URL in it,
				ending in something like <code>?rpc=</code>.
			</p>
			<p>You can manually enter the URL below:</p>
			<form method="get" action="">
				<input type="text" name="rpc" placeholder="RPC URL" />
				<input type="submit" />
			</form>
		</div>
	)
}

const NoToken: Component = () => {
	return (
		<div>
			<h1>Missing Token</h1>
			<p>
				The URL you opened should have contained a bearer token in it,
				ending in something like <code>?token=</code>.
			</p>
			<p>You can manually enter the token below:</p>
			<form method="get" action="">
				<input type="text" name="token" placeholder="Bearer token" />
				<input type="submit" />
			</form>
		</div>
	)
}

export const Loader: Component = () => {
	const params = new URLSearchParams(window.location.search)
	let rpcUrl = localStorage.getItem(rpcUrlKey)
	if (!rpcUrl) {
		rpcUrl = params.get('rpc')
		if (!rpcUrl) {
			return <NoRpc />
		}

		localStorage.setItem(rpcUrlKey, rpcUrl)
	}

	if (rpcUrl.startsWith('/')) {
		rpcUrl = window.location.origin + rpcUrl
	} else {
		try {
			new URL(rpcUrl)
		} catch (e) {
			return <NoRpc />
		}
	}

	let bearerToken = localStorage.getItem(bearerTokenKey)
	if (!bearerToken) {
		bearerToken = params.get('token')
		if (!bearerToken) {
			return <NoToken />
		}

		localStorage.setItem(bearerTokenKey, bearerToken)
	}

	const client = createClient(
		ClientRpcService,
		createConnectTransport({
			baseUrl: rpcUrl,
			interceptors: [
				((next) => async (req) => {
					req.header.set('Authorization', `Bearer ${bearerToken}`)
					return next(req)
				}) satisfies Interceptor,
			],
		}),
	)

	const clientInfo = createAsync(() => client.getClientInfo({}))

	return (
		<Suspense fallback={<div>Loading...</div>}>
			<ErrorBoundary
				fallback={<div>Failed to connect to client RPC</div>}
			>
				<Show when={clientInfo()}>
					<FileServerUrlCtx.Provider
						value={clientInfo()!.fileServerUrl}
					>
						<RpcClientCtx.Provider value={client}>
							<App />
						</RpcClientCtx.Provider>
					</FileServerUrlCtx.Provider>
				</Show>
			</ErrorBoundary>
		</Suspense>
	)
}
