'use strict'

import { createConnectTransport } from '@connectrpc/connect-web'
import { Code, ConnectError, createClient, type Interceptor } from '@connectrpc/connect'
import {
	type OnlineUserInfo,
	ServerRpcService,
} from '../pb/serverrpc/v1/rpc_pb.js'

function sleep(ms: number) {
	return new Promise<void>((res) => setTimeout(res, ms))
}

type RpcClient = ReturnType<typeof createClient<typeof ServerRpcService>>

class FriendNetServerWidget extends HTMLElement {
	readonly #loopInterval = 10_000
	readonly #timeout = 10_000

	readonly #roomName: string = ''
	readonly #rpc: RpcClient = undefined as unknown as RpcClient
	readonly #usersElem: HTMLDivElement = undefined as unknown as HTMLDivElement
	readonly #errMsgElem: HTMLDivElement =
		undefined as unknown as HTMLDivElement

	constructor() {
		super()

		const rpcUrl = this.getAttribute('rpc')
		const roomName = this.getAttribute('room')
		const label = this.getAttribute('label') || `FriendNet Status`
		const token = this.getAttribute('token')

		const shadow = this.attachShadow({ mode: 'open' })

		if (!rpcUrl) {
			shadow.appendChild(
				document.createTextNode('Missing RPC URL ("rpc" attribute)'),
			)
			return
		}
		if (!roomName) {
			shadow.appendChild(
				document.createTextNode('Missing room name ("room" attribute)'),
			)
			return
		}

		this.#roomName = roomName

		this.#rpc = createClient(
			ServerRpcService,
			createConnectTransport({
				baseUrl: rpcUrl,
				fetch: (
					input: RequestInfo | URL,
					init?: RequestInit,
				): Promise<Response> => {
					const abort = new AbortController()
					setTimeout(() => abort.abort(), this.#timeout)

					return fetch(input, {
						...init,
						signal: abort.signal,
					})
				},
				interceptors: [
					((next) => async (req) => {
						if (token) {
							req.header.set('Authorization', `Bearer ${token}`)
						}

						return next(req)
					}) satisfies Interceptor,
				],
			}),
		)

		const container = document.createElement('div')

		// creating the inner HTML of the editable list element
		container.innerHTML = `
<style>
	.container {
		width: 100%;
		height: 100%;
	}
</style>
<div class="container">
	<div class="label"></div>
	<div class="error-message"></div>
	<div class="users-container"></div>
</div>
		`

		const labelElem = container.getElementsByClassName(
			'label',
		)[0] as HTMLDivElement
		labelElem.textContent = label

		this.#usersElem = container.getElementsByClassName(
			'users-container',
		)[0] as HTMLDivElement
		this.#errMsgElem = container.getElementsByClassName(
			'error-message',
		)[0] as HTMLDivElement

		shadow.appendChild(container)
	}

	connectedCallback() {
		if (this.#roomName === '') {
			// Wasn't initialized properly.
			return
		}

		// noinspection JSIgnoredPromiseFromCall
		this.#loop()
	}

	async #loop() {
		await this.#loadUsersAndRender()

		while (this.isConnected) {
			await sleep(this.#loopInterval)
			if (!this.isConnected) {
				return
			}

			// Function handles its own errors.
			await this.#loadUsersAndRender()
		}
	}

	async #loadUsersAndRender() {
		try {
			const users: OnlineUserInfo[] = []

			const stream = this.#rpc.getOnlineUsers({ room: this.#roomName })
			for await (const res of stream) {
				for (const user of res.users) {
					users.push(user)
				}
			}

			this.#renderOnlineUsers(users)
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.Unauthenticated) {
					this.#errMsgElem.textContent =
						'Server requires authentication, but none was provided. You should use the "token" attribute to provide a bearer token.'
					return
				}
				if (err.code === Code.PermissionDenied) {
					this.#errMsgElem.textContent =
						'Server said permission denied: ' + err.message
					return
				}
			}

			console.error('failed to load online users:', err)
			this.#errMsgElem.textContent =
				'Failed to load online users: ' + String(err)
		}
	}

	#renderOnlineUsers(users: OnlineUserInfo[]) {
		this.#usersElem.innerHTML = ''
		for (const user of users) {
			const elem = document.createElement('div')
			elem.classList.add('online-user')
			elem.textContent = user.username
			this.#usersElem.appendChild(elem)
		}
	}
}

// let the browser know about the custom element
customElements.define('friendnet-server-widget', FriendNetServerWidget)
