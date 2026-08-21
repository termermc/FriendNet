import { Component, ErrorBoundary, JSX, Show, Suspense } from 'solid-js'
import { PageRoomCtx, usePageRoom, useRpcClient } from './ctx'
import { createAsync } from '@solidjs/router'
import { Code, ConnectError } from '@connectrpc/connect'
import stylesCommon from './common.module.css'

type RoomLoaderProps = {
	/**
	 * The room's name.
	 */
	room: string

	children: JSX.Element
}

/**
 * Loads a room and shows the children of the element when it is loaded.
 * The room can be accessed from {@link usePageRoom}, and is guaranteed to not be undefined.
 */
export const RoomLoader: Component<RoomLoaderProps> = (props) => {
	const client = useRpcClient()

	const room = createAsync(async () => {
		const { room } = await client.getRoomInfo({ name: props.room })
		return room!
	})

	return (
		<ErrorBoundary
			fallback={(err) => {
				if (err instanceof ConnectError) {
					if (err.code === Code.PermissionDenied) {
						return (
							<div class={stylesCommon.errorMessage}>
								The RPC method required to get room info is not
								available.
							</div>
						)
					}
					if (err.code === Code.NotFound) {
						return (
							<div class={stylesCommon.errorMessage}>
								Room not found.
							</div>
						)
					}
				}

				console.error(`failed to load room "${props.room}":`, err)

				return (
					<div class={stylesCommon.errorMessage}>
						Failed to load room "{props.room}", see console for
						details.
					</div>
				)
			}}
		>
			<Suspense fallback={<i>Loading...</i>}>
				<Show when={room()}>
					<PageRoomCtx.Provider value={room()}>
						{props.children}
					</PageRoomCtx.Provider>
				</Show>
			</Suspense>
		</ErrorBoundary>
	)
}
