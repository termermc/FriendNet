import { Component, ErrorBoundary, Show, Suspense } from 'solid-js'
import {
	bearerTokenKey,
	FileServerUrlCtx,
	GlobalStateCtx,
	RpcClientCtx,
	rpcUrlKey,
} from './ctx'
import App from './App'
import { createClient, Interceptor } from '@connectrpc/connect'
import {
	ClientRpcService,
	GetClientInfoResponse,
} from '../pb/clientrpc/v1/rpc_pb'
import { createConnectTransport } from '@connectrpc/connect-web'
import { createAsync } from '@solidjs/router'
import { State } from './state'
import { RpcTimeoutMs } from './constant'

const NoRpc: Component = () => {
	return (
		<div>
			<h1>Missing or Invalid RPC URL</h1>
			<p>
				The URL you opened should have contained an RPC URL in it,
				ending in something like <code>?rpc=</code>.
			</p>
			<p>
				You can also find it by searching "bearer_token" in the log
				output of your client process.
			</p>
			<p>You can manually enter the URL below:</p>
			<form method="get" action="">
				<input
					type="text"
					name="rpc"
					placeholder="https://localhost:20040"
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
			return <NoRpc />
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
		ClientRpcService,
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
		clientInfo: GetClientInfoResponse
		state: State
	}

	const everything = createAsync(async (): Promise<Everything> => {
		const clientInfo = await client.getClientInfo({})

		// Load initial state.
		const state = new State(client)
		await state.refreshServers()

		return { clientInfo, state }
	})

	return (
		<Suspense fallback={<div>Loading...</div>}>
			<ErrorBoundary
				fallback={
					<div>
						<p>Failed to connect to client RPC.</p>
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
				}
			>
				<Show when={everything()}>
					<FileServerUrlCtx.Provider
						value={everything()!.clientInfo.fileServerUrl}
					>
						<RpcClientCtx.Provider value={client}>
							<GlobalStateCtx.Provider
								value={everything()!.state}
							>
								<App />
							</GlobalStateCtx.Provider>
						</RpcClientCtx.Provider>
					</FileServerUrlCtx.Provider>
				</Show>
			</ErrorBoundary>
		</Suspense>
	)
}
