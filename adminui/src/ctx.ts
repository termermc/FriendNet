import { Accessor, createContext, createSignal, useContext } from 'solid-js'
import { GetServerInfoResponse, RoomInfo } from '../pb/serverrpc/v1/rpc_pb'
import { RpcClient } from './protobuf'

/**
 * The {@link localStorage} key to use for storing the bearer token.
 */
export const bearerTokenKey = 'friendnet.token'

/**
 * The {@link localStorage} key to use for storing the RPC URL.
 */
export const rpcUrlKey = 'friendnet.rpc'

export const ServerInfoCtx = createContext<GetServerInfoResponse>()
export const RpcClientCtx = createContext<RpcClient>()
export const RoomsCtx = createContext(createSignal<RoomInfo[]>([]))

/**
 * Returns information about the server.
 */
export function useServerInfo(): GetServerInfoResponse {
	return useContext(ServerInfoCtx)!
}

/**
 * Returns the app RPC client.
 */
export function useRpcClient(): RpcClient {
	return useContext(RpcClientCtx)!
}

/**
 * Returns a signal containing all loaded rooms.
 */
export function useRooms(): Accessor<RoomInfo[]> {
	const [getRooms] = useContext(RoomsCtx)!
	return getRooms
}

/**
 * Returns a function that refreshes the loaded rooms.
 */
export function useRefreshRooms(): () => Promise<void> {
	const client = useRpcClient()
	const [, setRooms] = useContext(RoomsCtx)!

	return async () => {
		const rooms = (await client.getRooms({})).rooms
		rooms.sort((a, b) => a.name.localeCompare(b.name))

		setRooms(rooms)
	}
}

/**
 * Returns a function that adds a room to the list of loaded rooms.
 */
export function useAddRoom(): (room: RoomInfo) => void {
	const [, setRooms] = useContext(RoomsCtx)!

	return (room: RoomInfo) => {
		setRooms((rooms) => [...rooms, room])
	}
}

/**
 * Returns a function that removes a room from the list of loaded rooms.
 */
export function useRemoveRoom(): (room: RoomInfo) => void {
	const [rooms, setRooms] = useContext(RoomsCtx)!

	return (room: RoomInfo) => {
		setRooms(rooms().filter((r) => r.name !== room.name))
	}
}
