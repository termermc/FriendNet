import { createContext, useContext } from 'solid-js'
import { Client } from '@connectrpc/connect'
import { ClientRpcService } from '../pb/clientrpc/v1/rpc_pb'
import { RpcClient } from './protobuf'
import { State } from './state'

/**
 * The {@link localStorage} key to use for storing the bearer token.
 */
export const bearerTokenKey = 'friendnet.token'

/**
 * The {@link localStorage} key to use for storing the RPC URL.
 */
export const rpcUrlKey = 'friendnet.rpc'

export const FileServerUrlCtx = createContext<string>()
export const RpcClientCtx = createContext<RpcClient>()
export const GlobalStateCtx = createContext<State>()

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

/**
 * Returns the app's global state.
 */
export function useGlobalState(): State {
	return useContext(GlobalStateCtx)!
}
