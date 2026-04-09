import { createContext, useContext } from 'solid-js'
import { GetServerInfoResponse } from '../pb/serverrpc/v1/rpc_pb'
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
