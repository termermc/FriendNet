import type { Client } from '@connectrpc/connect'
import type { ServerRpcService } from '../pb/serverrpc/v1/rpc_pb'

export type RpcClient = Client<typeof ServerRpcService>

export * from '../pb/serverrpc/v1/rpc_pb'
