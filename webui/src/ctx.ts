import { createContext, useContext } from 'solid-js'
import { Client } from '@connectrpc/connect'
import { ClientRpcService } from '../pb/clientrpc/v1/rpc_pb'

/**
 * The {@link localStorage} key to use for storing the bearer token.
 */
export const bearerTokenKey = 'friendnet.token'

/**
 * The {@link localStorage} key to use for storing the RPC URL.
 */
export const rpcUrlKey = 'friendnet.rpc'

export const FileServerUrlCtx = createContext<string>()
export const RpcClientCtx = createContext<Client<typeof ClientRpcService>>()

/**
 * Returns the base URL of the file server.
 */
export function useFileServerUrl(): string {
	return useContext(FileServerUrlCtx)!
}

/**
 * Returns the app RPC client.
 */
export function useRpcClient(): Client<typeof ClientRpcService> {
	return useContext(RpcClientCtx)!
}
