import { Accessor, createSignal, Setter } from 'solid-js'
import {
	CreateServerRequest,
	CreateShareRequest,
	Event,
	Event_Type,
	EventContext,
	LogMessage,
	LogMessageAttr,
	OnlineUserInfo,
	ServerConnState,
	ServerInfo,
	ShareInfo,
	UpdateServerRequest,
} from '../pb/clientrpc/v1/rpc_pb'
import { RpcClient } from './protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { sleep } from './util'

class Refresher {
	onlineUsers = new Set<string>()

	constructor(
		state: State,
		client: RpcClient,
		private readonly intervalMs: number,
	) {
		;(async () => {
			// noinspection InfiniteLoopJS
			while (true) {
				await sleep(this.intervalMs)
				for (const serverUuid of this.onlineUsers) {
					this.onlineUsers.delete(serverUuid)
					try {
						const server = state.getServerByUuid(serverUuid)
						if (server == null) {
							continue
						}

						const res = client.getOnlineUsers({ serverUuid })

						for await (const { users } of res) {
							for (const info of users) {
								server.setUserOnline(info)
							}
						}
					} catch (err) {
						console.error('failed to refresh online users', err)
					}
				}
			}
		})()
	}
}

/**
 * Represents an online user within a server room.
 */
export class OnlineUser {
	readonly username: string

	constructor(info: OnlineUserInfo) {
		this.username = info.username
	}

	updateFromInfo(_info: OnlineUserInfo): void {
		// Nothing to do for now.
	}
}

/**
 * Represents a server share.
 */
export class ServerShare {
	readonly name: string
	readonly path: string
	readonly createdTs: Date

	constructor(info: ShareInfo) {
		this.name = info.name
		this.path = info.path
		this.createdTs = new Date(Number(info.createdTs) * 1_000)
	}

	updateFromInfo(_info: ShareInfo): void {
		// Nothing to do for now.
	}
}

/**
 * Represents a FriendNet server.
 */
export class Server {
	readonly #client: RpcClient
	readonly #refresher: Refresher

	readonly uuid: string
	readonly createdTs: Date

	connState: Accessor<ServerConnState>
	#setConnState: Setter<ServerConnState>

	name: Accessor<string>
	#setName: Setter<string>

	address: Accessor<string>
	#setAddress: Setter<string>

	room: Accessor<string>
	#setRoom: Setter<string>

	username: Accessor<string>
	#setUsername: Setter<string>

	onlineUsers: Accessor<OnlineUser[]>
	#setOnlineUsers: Setter<OnlineUser[]>

	shares: Accessor<ServerShare[]>
	#setShares: Setter<ServerShare[]>

	constructor(client: RpcClient, refresher: Refresher, info: ServerInfo) {
		this.#client = client
		this.#refresher = refresher

		this.uuid = info.uuid
		this.createdTs = new Date(Number(info.createdTs) * 1_000)
		;[this.connState, this.#setConnState] = createSignal<ServerConnState>(
			ServerConnState.CLOSED,
		)
		;[this.name, this.#setName] = createSignal('')
		;[this.address, this.#setAddress] = createSignal('')
		;[this.room, this.#setRoom] = createSignal('')
		;[this.username, this.#setUsername] = createSignal('')
		;[this.onlineUsers, this.#setOnlineUsers] = createSignal<OnlineUser[]>(
			[],
		)
		;[this.shares, this.#setShares] = createSignal<ServerShare[]>([])

		this.updateFromInfo(info)
	}

	setConnState(state: ServerConnState): void {
		this.#setConnState(state)

		if (state === ServerConnState.OPEN) {
			this.refreshOnlineUsers()
		} else {
			this.#setOnlineUsers([])
		}
	}

	/**
	 * Sets a user as online.
	 * If the user is already online, it will be updated.
	 * If the user is not online, it will be added.
	 * @param info The online user's info.
	 */
	setUserOnline(info: OnlineUserInfo) {
		const cur = this.onlineUsers().find((x) => x.username === info.username)
		if (cur) {
			cur.updateFromInfo(info)
		} else {
			const newUsers = [...this.onlineUsers(), new OnlineUser(info)]
			newUsers.sort((a, b) => a.username.localeCompare(b.username))

			this.#setOnlineUsers(newUsers)
		}
	}

	/**
	 * Sets a user as offline.
	 * @param username The user's username.
	 */
	setUserOffline(username: string): void {
		const newUsers = this.onlineUsers().filter(
			(x) => x.username !== username,
		)
		this.#setOnlineUsers(newUsers)
	}

	/**
	 * Schedules a refresh of the online users.
	 */
	refreshOnlineUsers() {
		this.#refresher.onlineUsers.add(this.uuid)
	}

	async refreshShares(): Promise<void> {
		const res = await this.#client.getShares({ serverUuid: this.uuid })

		const curShares = this.shares()
		const newShares: ServerShare[] = []

		for (const info of res.shares) {
			const cur = curShares.find((x) => x.path === info.path)
			if (cur) {
				cur.updateFromInfo(info)
				newShares.push(cur)
			} else {
				newShares.push(new ServerShare(info))
			}
		}

		// Sort shares alphabetically.
		newShares.sort((a, b) => a.path.localeCompare(b.path))

		this.#setShares(newShares)
	}

	/**
	 * Creates a new share on the server.
	 * @param req The share creation request.
	 */
	async createShare(
		req: Omit<CreateShareRequest, '$typeName' | 'serverUuid'>,
	): Promise<void> {
		const { share } = await this.#client.createShare({
			serverUuid: this.uuid,
			...req,
		})
		this.#setShares([...this.shares(), new ServerShare(share!)])
	}

	/**
	 * Deletes the share with the specified name from the server.
	 * @param name The name of the share to delete.
	 * @returns Whether the share existed.
	 */
	async deleteShare(name: string): Promise<boolean> {
		try {
			await this.#client.deleteShare({ serverUuid: this.uuid, name })
		} catch (err) {
			if (err instanceof ConnectError && err.code === Code.NotFound) {
				return false
			}

			throw err
		}

		this.#setShares(this.shares().filter((x) => x.name !== name))
		return true
	}

	updateFromInfo(info: ServerInfo): void {
		this.setConnState(info.state!.connState)
		this.#setName(info.name)
		this.#setAddress(info.address)
		this.#setRoom(info.room)
		this.#setUsername(info.username)
	}

	/**
	 * Updates the server's info.
	 * @param req The values to update.
	 */
	async update(
		req: Omit<UpdateServerRequest, '$typeName' | 'uuid'>,
	): Promise<void> {
		await this.#client.updateServer({ uuid: this.uuid, ...req })

		if (req.name != null) {
			this.#setName(req.name)
		}
		if (req.address != null) {
			this.#setAddress(req.address)
		}
		if (req.room != null) {
			this.#setRoom(req.room)
		}
		if (req.username != null) {
			this.#setUsername(req.username)
		}
	}
}

/**
 * Information required to display a file preview.
 */
export type PreviewInfo = {
	serverUuid: string
	username: string
	path: string
}

/**
 * Manages streaming and querying client logs.
 * It automatically streams logs from the client RPC in the background and makes them available.
 */
export class LogManager {
	/**
	 * The latest log message.
	 * It is updated when new logs come in, so it is a good way to be notified of new messages.
	 */
	readonly latestLog: Accessor<LogMessage | undefined>
	readonly #setLatestLog: Setter<LogMessage | undefined>

	/**
	 * The total number of log messages currently in memory.
	 */
	readonly logCount: Accessor<number>
	readonly #setLogCount: Setter<number>

	#client: RpcClient

	#minBucket = -1
	#maxBucket = -1

	#bucketGranularity = 60 * 1_000
	#buckets = new Map<number, LogMessage[]>()

	constructor(client: RpcClient) {
		this.#client = client
		;[this.latestLog, this.#setLatestLog] = createSignal<
			LogMessage | undefined
		>()
		;[this.logCount, this.#setLogCount] = createSignal(0)

		// noinspection JSIgnoredPromiseFromCall
		this.#daemon()
	}

	attrsToObject(attrs: LogMessageAttr[]): Record<string, any> {
		const res: Record<string, any> = {}
		for (const attr of attrs) {
			switch (attr.kind) {
				case 'Bool':
				case 'Float64':
				case 'Int64':
				case 'Uint64':
					res[attr.key] = JSON.parse(attr.value)
					break
				default:
					res[attr.key] = attr.value
			}
		}
		return res
	}

	/**
	 * The daemon that streams messages.
	 * Should only be run once, by the constructor.
	 * It will never throw.
	 * @private
	 */
	async #daemon() {
		// Start with the last hour of logs.
		let lastMsgTs = BigInt(Date.now() - 60 * 60 * 1_000)

		// noinspection InfiniteLoopJS
		while (true) {
			const stream = this.#client.streamLogs({
				sendLogsAfterTs: lastMsgTs,
			})

			try {
				for await (const res of stream) {
					for (const msg of res.logs) {
						const wasDuplicate = this.#insert(msg, true)
						if (!wasDuplicate) {
							lastMsgTs = msg.createdTs
							this.#setLogCount(this.logCount() + 1)
							this.#setLatestLog(msg)

							// Log message to console.
							console.log(
								'[CLIENT]',
								msg.message,
								this.attrsToObject(msg.attrs),
							)
						}
					}
				}
			} catch (err) {
				console.error('error streaming logs:', err)
				await sleep(1_000)
			}
		}
	}

	#tsToBucketNum(ts: bigint | number): number {
		return Math.floor(Number(ts) / this.#bucketGranularity)
	}

	/**
	 * Inserts a new log message.
	 * @param msg The message.
	 * @param skipBucketSort Whether to skip sorting the bucket after insertion.
	 * This should be false unless you know that the inserted message will be the
	 * newest in the bucket (such as a newly streamed log message).
	 * @returns Whether the message already existed.
	 */
	#insert(msg: LogMessage, skipBucketSort: boolean): boolean {
		const bucketNum = this.#tsToBucketNum(msg.createdTs)
		const bucket = this.#buckets.get(bucketNum)
		if (bucket) {
			// Check if it already exists.
			for (const bucketMsg of bucket) {
				if (bucketMsg.uid === msg.uid) {
					// Skip inserting duplicate.
					return true
				}
			}

			bucket.push(msg)

			if (!skipBucketSort) {
				bucket.sort((a, b) => Number(a.createdTs - b.createdTs))
			}
		} else {
			// New bucket.
			this.#buckets.set(bucketNum, [msg])

			if (this.#minBucket === -1) {
				this.#minBucket = bucketNum
			}
			if (bucketNum > this.#maxBucket) {
				this.#maxBucket = bucketNum
			}
		}

		return false
	}

	/**
	 * Returns an iterator of log messages between min and max (both inclusive).
	 * @param min The minimum timestamp.
	 * @param max The maximum timestamp.
	 * @returns All log messages between min and max (both inclusive).
	 */
	*iterateRange(min: Date, max: Date): Generator<LogMessage, void, void> {
		if (this.#minBucket === -1 || this.#maxBucket === -1) {
			return
		}

		const minBucketNum = Math.max(
			this.#tsToBucketNum(min.getTime()),
			this.#minBucket,
		)
		const maxBucketNum = Math.min(
			this.#tsToBucketNum(max.getTime()),
			this.#maxBucket,
		)

		for (let i = minBucketNum; i <= maxBucketNum; i++) {
			const bucket = this.#buckets.get(i)
			if (bucket == null) {
				continue
			}

			for (const msg of bucket) {
				yield msg
			}
		}
	}

	/**
	 * Returns an iterator of log messages after the specified timestamp.
	 * @param after The timestamp to start iterating from.
	 * @returns All log messages after the timestamp.
	 */
	*iterateAfter(after: Date): Generator<LogMessage, void, void> {
		if (this.#minBucket === -1 || this.#maxBucket === -1) {
			return
		}

		let bucketNum = Math.max(
			this.#minBucket,
			this.#tsToBucketNum(after.getTime()),
		)

		let count = 0
		while (bucketNum <= this.#maxBucket) {
			const bucket = this.#buckets.get(bucketNum)
			bucketNum++
			if (bucket == null) {
				continue
			}

			for (let i = 0; i < bucket.length; i++) {
				yield bucket[i]
				count++
			}
		}
	}

	/**
	 * Returns an iterator of log messages backwards before the specified timestamp.
	 * @param before The timestamp to start iterating backwards from.
	 * @returns All log messages backwards before the timestamp.
	 */
	*iterateBefore(before: Date): Generator<LogMessage, void, void> {
		if (this.#minBucket === -1 || this.#maxBucket === -1) {
			return
		}

		let bucketNum = Math.min(
			this.#maxBucket,
			this.#tsToBucketNum(before.getTime()),
		)

		let count = 0
		while (bucketNum >= this.#minBucket) {
			const bucket = this.#buckets.get(bucketNum)
			bucketNum--
			if (bucket == null) {
				continue
			}

			for (let i = 0; i < bucket.length; i++) {
				yield bucket[bucket.length - i - 1]
				count++
			}
		}
	}
}

/**
 * EventListener is a listener for events from the client RPC.
 */
export type EventListener = (event: Event, ctx: EventContext) => void

/**
 * EventManager manages streaming events from the client and distributing them to subscribers.
 */
export class EventManager {
	#state: State
	#client: RpcClient

	#subscribers = new Map<Event_Type, Set<EventListener>>()

	/**
	 * Adds a new event listener for the specified event type.
	 * @param type The event type to listen to.
	 * @param listener The listener.
	 */
	addEventListener(type: Event_Type, listener: EventListener): void {
		let subscribers = this.#subscribers.get(type)
		if (subscribers == null) {
			subscribers = new Set()
			this.#subscribers.set(type, subscribers)
		}

		subscribers.add(listener)
	}

	/**
	 * Removes an event listener for the specified event type.
	 * @param type The event type to remove the listener from.
	 * @param listener The listener.
	 */
	removeEventListener(type: Event_Type, listener: EventListener): void {
		const subscribers = this.#subscribers.get(type)
		if (subscribers == null) {
			return
		}

		subscribers.delete(listener)
	}

	constructor(state: State, client: RpcClient) {
		this.#state = state
		this.#client = client

		// noinspection JSIgnoredPromiseFromCall
		this.#daemon()
	}

	/**
	 * The daemon that streams events.
	 * Should only be run once, by the constructor.
	 * It will never throw.
	 * @private
	 */
	async #daemon() {
		// noinspection InfiniteLoopJS
		while (true) {
			const stream = this.#client.streamEvents({})

			try {
				for await (const res of stream) {
					console.log(
						'[EVENT]',
						Event_Type[res.event!.type],
						res.event,
						res.context,
					)

					const evt = res.event!

					const subs = this.#subscribers.get(evt.type)
					if (subs == null) {
						continue
					}

					for (const sub of subs) {
						try {
							sub(evt, res.context!)
						} catch (err) {
							console.error(
								`error in event listener for type ${Event_Type[evt.type]} (${evt.type}):`,
								err,
							)
						}
					}
				}
			} catch (err) {
				console.error('error streaming events:', err)
				await sleep(1_000)
			}

			// Try to refresh state before reconnecting.
			try {
				await this.#state.doFullRefresh()
			} catch (err) {
				console.error(
					'failed to refresh state after event stream error:',
					err,
				)
			}
		}
	}
}

export class State {
	readonly #client: RpcClient

	/**
	 * The global {@link LogManager} instance.
	 */
	readonly log: LogManager

	/**
	 * The global {@link EventManager} instance.
	 */
	readonly event: EventManager

	/**
	 * The global {@link Refresher} instance.
	 * Used to schedule refreshes of various resources.
	 */
	readonly refresher: Refresher

	readonly previewInfo: Accessor<PreviewInfo | undefined>
	readonly #setPreviewInfo: Setter<PreviewInfo | undefined>

	readonly servers: Accessor<Server[]>
	readonly #setServers: Setter<Server[]>

	constructor(client: RpcClient) {
		this.#client = client

		this.log = new LogManager(client)
		this.event = new EventManager(this, client)
		this.refresher = new Refresher(this, client, 500)

		;[this.servers, this.#setServers] = createSignal<Server[]>([])
		;[this.previewInfo, this.#setPreviewInfo] = createSignal<
			PreviewInfo | undefined
		>()

		// Listen to server events.
		this.event.addEventListener(Event_Type.STOP, () => {
			window.close()
			setTimeout(() => window.location.assign('about:blank'), 100)
		})
		this.event.addEventListener(
			Event_Type.SERVER_CONN_STATE_CHANGE,
			(event, ctx) => {
				const server = this.getServerByUuid(ctx.serverUuid)
				if (server == null) {
					return
				}

				server.setConnState(event.serverConn!.state)
			},
		)
		this.event.addEventListener(Event_Type.CLIENT_ONLINE, (event, ctx) => {
			const server = this.getServerByUuid(ctx.serverUuid)
			if (server == null) {
				return
			}

			server.setUserOnline(event.clientOnline!.info!)
		})
		this.event.addEventListener(Event_Type.CLIENT_OFFLINE, (event, ctx) => {
			const server = this.getServerByUuid(ctx.serverUuid)
			if (server == null) {
				return
			}

			server.setUserOffline(event.clientOffline!.username)
		})

		// Periodically refresh state.
		setInterval(() => {
			this.doFullRefresh().catch((err) =>
				console.error('error doing periodic full state refresh:', err),
			)
		}, 10_000)
	}

	/**
	 * Does a full state refresh.
	 */
	async doFullRefresh() {
		await this.refreshServers()

		for (const server of this.servers()) {
			server.refreshOnlineUsers()
		}
	}

	/**
	 * Sets a file to be previewed.
	 * @param serverUuid The UUID of the server the file is exposed through.
	 * @param username The username of the user hosting the file.
	 * @param path The file's path.
	 */
	previewFile(serverUuid: string, username: string, path: string): void {
		const cur = this.previewInfo()
		if (
			cur != null &&
			cur.serverUuid === serverUuid &&
			cur.username === username &&
			cur.path === path
		) {
			return
		}

		this.#setPreviewInfo({
			serverUuid,
			username,
			path,
		})
	}

	/**
	 * Closes the preview viewer, if open.
	 */
	closePreview(): void {
		this.#setPreviewInfo(undefined)
	}

	/**
	 * Returns the server with the specified UUID, or undefined if not found.
	 * @param uuid The UUID of the server to find.
	 * @returns The server, or undefined if not found.
	 */
	getServerByUuid(uuid: string): Server | undefined {
		return this.servers().find((x) => x.uuid === uuid)
	}

	/**
	 * Refreshes the list of servers.
	 * Any existing servers whose information was updated will be updated in-place.
	 */
	async refreshServers(): Promise<void> {
		const { servers } = await this.#client.getServers({})

		const curServers = this.servers()
		const newServers: Server[] = []

		for (const info of servers) {
			const cur = curServers.find((x) => x.uuid === info.uuid)
			if (cur) {
				cur.updateFromInfo(info)
				newServers.push(cur)
			} else {
				newServers.push(new Server(this.#client, this.refresher, info))
			}
		}

		// Sort by name.
		newServers.sort((a, b) => a.name().localeCompare(b.name()))

		this.#setServers(newServers)
	}

	/**
	 * Creates a new server and adds it to the list.
	 * @param req The create server request.
	 * @returns The newly created server's UUID.
	 */
	async createServer(
		req: Omit<CreateServerRequest, '$typeName'>,
	): Promise<string> {
		const res = await this.#client.createServer(req)

		this.#setServers([
			...this.servers(),
			new Server(this.#client, this.refresher, res.server!),
		])

		return res.server!.uuid
	}

	/**
	 * Deletes the server with the specified UUID from the list.
	 * @param uuid The UUID of the server to delete.
	 * @returns Whether the server existed.
	 */
	async deleteServer(uuid: string): Promise<boolean> {
		try {
			await this.#client.deleteServer({ uuid })
			this.#setServers(this.servers().filter((x) => x.uuid !== uuid))
			return true
		} catch (err) {
			if (err instanceof ConnectError && err.code === Code.NotFound) {
				return false
			}

			throw err
		}
	}
}
