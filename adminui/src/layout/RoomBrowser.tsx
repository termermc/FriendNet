import styles from './RoomBrowser.module.css'
import stylesCommon from '../common.module.css'

import { Component, ErrorBoundary, For, Show, Suspense } from 'solid-js'
import { useRefreshRooms, useRooms } from '../ctx'
import { A, createAsync } from '@solidjs/router'
import { Code, ConnectError } from '@connectrpc/connect'
import { RoomInfo } from '../../pb/serverrpc/v1/rpc_pb'

const Room: Component<{ room: RoomInfo }> = (props) => {
	return (
		<A class={styles.room} href={`/room/${props.room.name}`}>
			<div class={styles.roomName}>🚪 {props.room.name}</div>
			<div
				title={`Online users: ${props.room.onlineUserCount}`}
				class={styles.onlineUserCount}
			>
				🛜 {props.room.onlineUserCount}
			</div>
		</A>
	)
}

export const RoomBrowser: Component = () => {
	const refreshRooms = useRefreshRooms()
	const rooms = useRooms()

	const initialLoad = createAsync(async () => {
		await refreshRooms()
		return true
	})

	return (
		<div class={styles.container}>
			<ErrorBoundary
				fallback={(err) => {
					if (
						err instanceof ConnectError &&
						err.code === Code.PermissionDenied
					) {
						return (
							<div class={stylesCommon.errorMessage}>
								The RPC method required to list rooms is not
								available.
							</div>
						)
					}

					console.error('failed to load rooms:', err)

					return (
						<div class={stylesCommon.errorMessage}>
							Failed to load rooms, see console for details.
						</div>
					)
				}}
			>
				<Suspense fallback={<i>Loading...</i>}>
					{initialLoad()}

					<Show
						when={rooms().length > 0}
						fallback={
							<p>
								No rooms yet.{' '}
								<A href="/createroom">Create one</A>.
							</p>
						}
					>
						<For each={rooms()}>
							{(room) => <Room room={room} />}
						</For>
					</Show>
				</Suspense>
			</ErrorBoundary>
		</div>
	)
}
