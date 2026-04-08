import { createContext, useContext } from 'solid-js'
import { Client } from '@connectrpc/connect'
import { ClientRpcService } from '../pb/serverrpc/v1/rpc_pb'
import { RpcClient } from './protobuf'

/**
 * The {@link localStorage} key to use for storing the bearer token.
 */
export const bearerTokenKey = 'friendnet.token'

/**
 * The {@link localStorage} key to use for storing the RPC URL.
 */
export const rpcUrlKey = 'friendnet.rpc'

export const RpcClientCtx = createContext<RpcClient>()

/**
 * Returns the app RPC client.
 */
export function useRpcClient(): Client<typeof ClientRpcService> {
	return useContext(RpcClientCtx)!
}
