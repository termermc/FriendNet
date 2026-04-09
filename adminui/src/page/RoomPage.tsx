import styles from './RoomPage.module.css'

import {
	Component,
	createSignal,
	ErrorBoundary,
	For,
	onCleanup,
	onMount,
	Show,
	Suspense,
} from 'solid-js'

import {
	createAsync,
	useLocation,
	useNavigate,
	useParams,
} from '@solidjs/router'
import { Code, ConnectError } from '@connectrpc/connect'
import { useRemoveRoom, useRpcClient } from '../ctx'
import { OnlineUserInfo, RoomInfo } from '../../pb/serverrpc/v1/rpc_pb'
import stylesCommon from '../common.module.css'

const Page: Component<{ room: RoomInfo }> = (props) => {
	const room = props.room

	const client = useRpcClient()
	const navigate = useNavigate()
	const removeRoom = useRemoveRoom()

	const [isRemoving, setRemoving] = createSignal(false)
	const [removeError, setRemoveError] = createSignal('')
	const remove = async () => {
		if (isRemoving()) {
			return
		}

		const confirmName = prompt(
			`Are you sure? Deleting the room will delete all its accounts and kick out all connected users.\n\nType the room's name to confirm:`,
		)
		if (confirmName?.toLowerCase() !== room.name.toLowerCase()) {
			return
		}

		try {
			setRemoving(true)

			await client.deleteRoom({ name: room.name })

			removeRoom(room)
			navigate('/')
		} catch (err) {
			if (err instanceof ConnectError) {
				setRemoveError(err.message)
				return
			}

			console.error('failed to remove room:', err)

			setRemoveError('Internal error, check console')
		} finally {
			setRemoving(false)
		}
	}

	const [onlineUsers, setOnlineUsers] = createSignal<OnlineUserInfo[]>([])
	const onlineInitialLoad = createAsync(async () => {
		for await (const page of client.getOnlineUsers({ room: room.name })) {
			setOnlineUsers((prev) =>
				[...prev, ...page.users].sort((a, b) =>
					a.username.localeCompare(b.username),
				),
			)
		}
		return true
	})
	let onlineRefreshInterval = 0
	onMount(() => {
		onlineRefreshInterval = +setInterval(async () => {
			try {
				const users: OnlineUserInfo[] = []
				for await (const page of client.getOnlineUsers({
					room: room.name,
				})) {
					users.push(...page.users)
				}
				users.sort((a, b) => a.username.localeCompare(b.username))
				setOnlineUsers(users)
			} catch (err) {
				console.error('failed to refresh online users:', err)
			}
		}, 10_000)
	})
	onCleanup(() => clearInterval(onlineRefreshInterval))

	return (
		<div class={styles.container}>
			<div class={styles.main}>
				<h1>Room: {room.name}</h1>
				<button
					onClick={remove}
					disabled={isRemoving()}
				>
					Delete Room
				</button>
			</div>

			<div class={styles.sidebar}>
				<h1>🛜 Online</h1>
				<div class={styles.onlineUsers}>
					<ErrorBoundary
						fallback={(err) => {
							if (
								err instanceof ConnectError &&
								err.code === Code.PermissionDenied
							) {
								return (
									<div class={stylesCommon.errorMessage}>
										The RPC method required to list online
										users is not available.
									</div>
								)
							}

							console.error('failed to load online users:', err)

							return (
								<div class={stylesCommon.errorMessage}>
									Failed to load online users, see console for
									details.
								</div>
							)
						}}
					>
						<Suspense fallback={<i>Loading...</i>}>
							{onlineInitialLoad()}

							<Show
								when={onlineUsers().length > 0}
								fallback={<i>No online users.</i>}
							>
								<For each={onlineUsers()}>
									{(user) => (
										<div
											class={styles.onlineUser}
										>
											<div class={styles.onlineUserStatus} />
											<span>{user.username}</span>
										</div>
									)}
								</For>
							</Show>
						</Suspense>
					</ErrorBoundary>
				</div>
			</div>
		</div>
	)
}

export const Loader: Component = () => {
	const { name } = useParams<{ name: string }>()
	const client = useRpcClient()

	const room = createAsync(async () => {
		const { room } = await client.getRoomInfo({ name })
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

				console.error('failed to load room:', err)

				return (
					<div class={stylesCommon.errorMessage}>
						Failed to load room, see console for details.
					</div>
				)
			}}
		>
			<Suspense fallback={<i>Loading...</i>}>
				<Show when={room()}>
					<Page room={room()!} />
				</Show>
			</Suspense>
		</ErrorBoundary>
	)
}

export const RoomPage: Component = () => {
	const loc = useLocation()

	return (
		<Show when={loc.pathname} keyed>
			<Loader />
		</Show>
	)
}
