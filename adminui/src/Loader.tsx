import { Component, ErrorBoundary, Show, Suspense } from 'solid-js'
import { bearerTokenKey, RpcClientCtx, rpcUrlKey, ServerInfoCtx } from './ctx'
import App from './App'
import {
	Code,
	ConnectError,
	createClient,
	Interceptor,
} from '@connectrpc/connect'
import {
	ServerRpcService,
	GetServerInfoResponse,
} from '../pb/serverrpc/v1/rpc_pb'
import { createConnectTransport } from '@connectrpc/connect-web'
import { createAsync } from '@solidjs/router'
import { RpcTimeoutMs } from './constant'

const NoRpc: Component = () => {
	return (
		<div>
			<h1>Invalid RPC URL</h1>
			<p>You can manually enter the URL below:</p>
			<form method="get" action="">
				<input
					type="text"
					name="rpc"
					placeholder="https://127.0.0.1:20042"
				/>
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
	let rpcUrl = params.get('rpc')
	if (rpcUrl) {
		localStorage.setItem(rpcUrlKey, rpcUrl)
	} else {
		rpcUrl = localStorage.getItem(rpcUrlKey)
		if (!rpcUrl) {
			rpcUrl = window.location.origin
		}
	}

	if (rpcUrl.startsWith('/')) {
		rpcUrl = window.location.origin + rpcUrl
	} else if (
		!rpcUrl.startsWith('http://') &&
		!rpcUrl.startsWith('https://')
	) {
		return <NoRpc />
	} else {
		try {
			new URL(rpcUrl)
		} catch (e) {
			return <NoRpc />
		}
	}

	let bearerToken = params.get('token')
	if (bearerToken) {
		localStorage.setItem(bearerTokenKey, bearerToken)
	} else {
		bearerToken = localStorage.getItem(bearerTokenKey)
		if (!bearerToken) {
			return <NoToken />
		}
	}

	// Clear out params from query.
	{
		params.delete('rpc')
		params.delete('token')
		let newUrl = window.location.origin + window.location.pathname
		const newParamStr = params.toString()
		if (newParamStr) {
			newUrl += `?${newParamStr}`
		}
		newUrl += window.location.hash
		window.history.replaceState({}, '', newUrl)
	}

	const client = createClient(
		ServerRpcService,
		createConnectTransport({
			fetch: (
				input: RequestInfo | URL,
				init?: RequestInit,
			): Promise<Response> => {
				let url: URL
				if (input instanceof Request) {
					url = new URL(input.url)
				} else if (typeof input === 'string') {
					url = new URL(input)
				} else if (input instanceof URL) {
					url = input
				} else {
					throw new Error('Invalid input type for fetch')
				}

				let signal: AbortSignal | undefined
				// Do not time out stream methods.
				if (!url.pathname.includes('Stream')) {
					const abort = new AbortController()
					setTimeout(() => abort.abort(), RpcTimeoutMs)
					signal = abort.signal
				}

				return fetch(input, {
					...init,
					signal,
				})
			},
			baseUrl: rpcUrl,
			interceptors: [
				((next) => async (req) => {
					req.header.set('Authorization', `Bearer ${bearerToken}`)
					return next(req)
				}) satisfies Interceptor,
			],
		}),
	)

	type Everything = {
		serverInfo: GetServerInfoResponse
	}

	const everything = createAsync(async (): Promise<Everything> => {
		const serverInfo = await client.getServerInfo({})

		if (!serverInfo.rpc!.requiresBearerToken) {
			throw new Error(
				'RPC does not require a bearer token. Exposing an administrative RPC interface without requiring a bearer token is dangerous! For security, the admin UI refuses to use this RPC interface until bearer token authentication is configured.',
			)
		}

		return {
			serverInfo,
		}
	})

	return (
		<Suspense fallback={<div>Loading...</div>}>
			<ErrorBoundary
				fallback={(err) => {
					if (
						err instanceof ConnectError &&
						err.code === Code.PermissionDenied
					) {
						return (
							<div>
								<p>
									Permission denied. Either your token is
									invalid or the RPC interface does not allow
									the <code>GetServerInfo</code> method.
								</p>
								<button
									onClick={() => {
										localStorage.removeItem(bearerTokenKey)
										window.location.reload()
									}}
								>
									Clear token.
								</button>
							</div>
						)
					}

					return (
						<div>
							<p>
								Failed to connect to server RPC:
								<br />
								<br />
								{err?.message ?? String(err)}
							</p>
							<button
								onClick={() => {
									localStorage.removeItem(rpcUrlKey)
									localStorage.removeItem(bearerTokenKey)
									window.location.reload()
								}}
							>
								Clear RPC URL and token.
							</button>
						</div>
					)
				}}
			>
				<Show when={everything()}>
					<ServerInfoCtx.Provider value={everything()!.serverInfo}>
						<RpcClientCtx.Provider value={client}>
							<App />
						</RpcClientCtx.Provider>
					</ServerInfoCtx.Provider>
				</Show>
			</ErrorBoundary>
		</Suspense>
	)
}
