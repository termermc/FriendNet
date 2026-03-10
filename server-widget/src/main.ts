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
	static tag = 'friendnet-server-widget'

	readonly #loopInterval = 10_000
	readonly #timeout = 10_000

	readonly #roomName: string = ''
	readonly #rpc: RpcClient = undefined as unknown as RpcClient
	readonly #usersElem: HTMLDivElement = undefined as unknown as HTMLDivElement
	readonly #errMsgElem: HTMLDivElement =
		undefined as unknown as HTMLDivElement
	readonly #roomInfoElem: HTMLDivElement =
		undefined as unknown as HTMLDivElement
	readonly #roomNameElem: HTMLSpanElement =
		undefined as unknown as HTMLSpanElement
	readonly #roomUserCountElem: HTMLSpanElement =
		undefined as unknown as HTMLSpanElement

	constructor() {
		super()

		const rpcUrl = this.getAttribute('rpc')
		const roomName = this.getAttribute('room')
		const label = this.getAttribute('label') || `FriendNet Server`
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
	:host, :host > div {
		display: block;
		width: 100%;
		height: 100%;
	}
	.container {
		width: 100%;
		height: 100%;
		background-color: #282c34;
		font-family: sans-serif;
		color: white;
	}
	.label {
		font-weight: bold;
		text-align: center;
		background-color: rgba(0, 0, 0, 0.25);
		padding: 0.25rem;
	}
	.room-info {
		font-size: 0.9rem;
		font-weight: bold;
		padding: 0.25rem;
		margin-bottom: 0.25rem;
		border-bottom: 0.2rem solid rgba(0, 0, 0, 0.25);
	}
	.users-container {
		overflow: auto;
		max-height: calc(100% - 2.5rem);
	}
	.online-user {
		background-color: rgba(0, 255, 0, 0.25);
		font-weight: bold;
		cursor: default;
		margin-bottom: 0.25rem;
	}
	.online-user::before {
		content: ' • ';
		color: lime;
		margin-left: 0.5rem;
	}
</style>
<div class="container">
	<div class="label"></div>
	<div class="error-message"></div>
	<div class="room-info">
		Room: <span class="room-name"></span>
		(<span class="room-user-count"></span>)
	</div>
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
		this.#roomInfoElem = container.getElementsByClassName(
			'room-info',
		)[0] as HTMLDivElement
		this.#roomNameElem = container.getElementsByClassName(
			'room-name',
		)[0] as HTMLSpanElement
		this.#roomUserCountElem = container.getElementsByClassName(
			'room-user-count',
		)[0] as HTMLSpanElement

		this.#roomInfoElem.style.display = 'none'

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
			const { room } = await this.#rpc.getRoomInfo({ name: this.#roomName })
			if (!room) {
				this.#errMsgElem.textContent = `Room "${this.#roomName}" not found`
				this.#usersElem.innerHTML = ''
				return
			}

			this.#roomInfoElem.style.display = 'block'
			this.#roomNameElem.textContent = room.name
			this.#roomUserCountElem.textContent = String(room.onlineUserCount)

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

customElements.define(FriendNetServerWidget.tag, FriendNetServerWidget)
