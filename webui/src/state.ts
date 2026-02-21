import { Accessor, createSignal, Setter } from 'solid-js'
import {
	CreateServerRequest,
	OnlineUserInfo,
	ServerInfo,
	UpdateServerRequest,
} from '../pb/clientrpc/v1/rpc_pb'
import { RpcClient } from './protobuf'
import { Code, ConnectError } from '@connectrpc/connect'

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
 * Represents a FriendNet server.
 */
export class Server {
	readonly uuid: string
	readonly createdTs: Date

	label: Accessor<string>
	#setLabel: Setter<string>

	address: Accessor<string>
	#setAddress: Setter<string>

	room: Accessor<string>
	#setRoom: Setter<string>

	username: Accessor<string>
	#setUsername: Setter<string>

	onlineUsers: Accessor<OnlineUser[]>
	#setOnlineUsers: Setter<OnlineUser[]>

	constructor(info: ServerInfo) {
		this.uuid = info.uuid
		this.createdTs = new Date(Number(info.createdTs) * 1_000)
		;[this.label, this.#setLabel] = createSignal('')
		;[this.address, this.#setAddress] = createSignal('')
		;[this.room, this.#setRoom] = createSignal('')
		;[this.username, this.#setUsername] = createSignal('')
		;[this.onlineUsers, this.#setOnlineUsers] = createSignal<OnlineUser[]>(
			[],
		)

		this.updateFromInfo(info)
	}

	async refreshOnlineUsers(client: RpcClient): Promise<void> {
		const res = client.getOnlineUsers({ serverUuid: this.uuid })

		const curUsers = this.onlineUsers()
		const newUsers: OnlineUser[] = []

		for await (const { users } of res) {
			for (const info of users) {
				const cur = curUsers.find((x) => x.username === info.username)
				if (cur) {
					cur.updateFromInfo(info)
					newUsers.push(cur)
				} else {
					newUsers.push(new OnlineUser(info))
				}
			}
		}

		// Sort users alphabetically.
		newUsers.sort((a, b) => a.username.localeCompare(b.username))

		this.#setOnlineUsers(newUsers)
	}

	updateFromInfo(info: ServerInfo): void {
		this.#setLabel(info.name)
		this.#setAddress(info.address)
		this.#setRoom(info.room)
		this.#setUsername(info.username)
	}

	/**
	 * Updates the server's info.
	 * @param client The RPC client to use.
	 * @param req The values to update.
	 */
	async update(
		client: RpcClient,
		req: Omit<UpdateServerRequest, '$typeName' | 'uuid'>,
	): Promise<void> {
		await client.updateServer({ uuid: this.uuid, ...req })

		if (req.name != null) {
			this.#setLabel(req.name)
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

export class State {
	servers: Accessor<Server[]>
	#setServers: Setter<Server[]>

	constructor() {
		;[this.servers, this.#setServers] = createSignal<Server[]>([])
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
	 * @param client The RPC client to use.
	 */
	async refreshServers(client: RpcClient): Promise<void> {
		const { servers } = await client.getServers({})

		const curServers = this.servers()
		const newServers: Server[] = []

		for (const info of servers) {
			const cur = curServers.find((x) => x.uuid === info.uuid)
			if (cur) {
				cur.updateFromInfo(info)
				newServers.push(cur)
			} else {
				newServers.push(new Server(info))
			}
		}

		// Sort by name.
		newServers.sort((a, b) => a.label().localeCompare(b.label()))

		this.#setServers(newServers)
	}

	/**
	 * Creates a new server and adds it to the list.
	 * @param client The RPC client to use.
	 * @param req The create server request.
	 * @returns The newly created server's UUID.
	 */
	async createServer(
		client: RpcClient,
		req: Omit<CreateServerRequest, '$typeName'>,
	): Promise<string> {
		const res = await client.createServer(req)

		this.#setServers([...this.servers(), new Server(res.server!)])

		return res.server!.uuid
	}

	/**
	 * Deletes the server with the specified UUID from the list.
	 * @param client The RPC client to use.
	 * @param uuid The UUID of the server to delete.
	 * @returns Whether the server existed.
	 */
	async deleteServer(client: RpcClient, uuid: string): Promise<boolean> {
		try {
			await client.deleteServer({ uuid })
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
