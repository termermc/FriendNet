import type { Client } from '@connectrpc/connect'
import type { ClientRpcService } from '../pb/clientrpc/v1/rpc_pb'

export type RpcClient = Client<typeof ClientRpcService>

export * from '../pb/clientrpc/v1/rpc_pb'
