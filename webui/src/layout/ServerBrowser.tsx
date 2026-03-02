import {
	Component,
	createEffect,
	createSignal,
	For,
	onCleanup,
	onMount,
	Show,
} from 'solid-js'
import { useGlobalState } from '../ctx'

import styles from './ServerBrowser.module.css'
import stylesCommon from '../common.module.css'

import { OnlineUser, Server } from '../state'
import { A } from '@solidjs/router'
import { Code, ConnectError } from '@connectrpc/connect'
import { makeBrowsePath, sleep } from '../util'
import { ServerConnState } from '../../pb/clientrpc/v1/rpc_pb'

const indicatorTitle = {
	[ServerConnState.UNSPECIFIED]: 'Disconnected',
	[ServerConnState.CLOSED]: 'Disconnected',
	[ServerConnState.OPENING]: 'Connecting...',
	[ServerConnState.OPEN]: 'Connected',
}
const indicatorClasses = {
	[ServerConnState.UNSPECIFIED]: styles.closed,
	[ServerConnState.CLOSED]: styles.closed,
	[ServerConnState.OPENING]: styles.opening,
	[ServerConnState.OPEN]: styles.open,
}

/**
 * A server connection state indicator.
 */
const ConnStateIndicator: Component<{ state: ServerConnState }> = (props) => {
	return (
		<div
			title={indicatorTitle[props.state]}
			classList={{
				[styles.serverConnState]: true,
				[indicatorClasses[props.state]]: true,
			}}
		/>
	)
}

const OnlineUserEntry: Component<{ server: Server; user: OnlineUser }> = (
	props,
) => {
	return (
		<div class={styles.onlineUser}>
			<div class={styles.onlineUserName}>
				<div class={styles.onlineUserStatus} />
				<span>{props.user.username}</span>
			</div>
			<div class={styles.onlineUserOptions}>
				<A
					href={makeBrowsePath(
						props.server.uuid,
						props.user.username,
						'',
					)}
				>
					📂 Browse
				</A>
				<A
					href={`/server/${props.server.uuid}/profile/${props.user.username}`}
				>
					👤 Profile
				</A>
			</div>
		</div>
	)
}

const ServerEntry: Component<{ server: Server }> = (props) => {
	const state = useGlobalState()

	const refreshUsers = async () => {
		try {
			await props.server.refreshOnlineUsers()
		} catch (err) {
			if (err instanceof ConnectError && err.code === Code.NotFound) {
				// Server was probably deleted.
				state.refreshServers().catch((err) => {
					console.error(
						'failed to refresh servers after apparent server deletion:',
						err,
					)
				})
				return
			}

			console.error('failed to refresh online users:', err)
		}
	}

	let runRefresher = true
	;(async () => {
		while (runRefresher) {
			await sleep(30_000)
			await refreshUsers()
		}
	})()
	onCleanup(() => (runRefresher = false))

	const [isDeleting, setDeleting] = createSignal(false)
	const doDelete = async () => {
		if (isDeleting()) {
			return
		}

		if (
			!confirm(
				`Are you sure you want to delete ${JSON.stringify(props.server.name())}?`,
			)
		) {
			return
		}

		try {
			setDeleting(true)
			await state.deleteServer(props.server.uuid)
		} catch (err) {
			console.error(
				`failed to delete server ${JSON.stringify(props.server.uuid)}:`,
				err,
			)
			alert(
				`Failed to delete server ${props.server.uuid}, see console for details`,
			)
		} finally {
			setDeleting(false)
		}
	}

	const [isPendingStateChange, setPendingStateChange] = createSignal(false)
	createEffect(() => {
		props.server.connState()
		setPendingStateChange(false)
	})

	const doConnect = async (e: Event) => {
		e.preventDefault()

		if (isPendingStateChange()) {
			return
		}

		setPendingStateChange(true)
		await props.server.connect()
	}
	const doDisconnect = async (e: Event) => {
		e.preventDefault()

		if (isPendingStateChange()) {
			return
		}

		setPendingStateChange(true)
		await props.server.disconnect()
	}

	return (
		<details
			open={true}
			classList={{
				[styles.server]: true,
				[stylesCommon.sidebarContainer]: true,
			}}
		>
			<summary>
				<span title={props.server.name()}>
					<ConnStateIndicator state={props.server.connState()} />
					{props.server.name()}
				</span>

				<A
					title="Edit Server"
					class={stylesCommon.action}
					href={`/server/${props.server.uuid}/edit`}
				>
					📝️
				</A>
				<button
					title="Delete Server"
					onClick={doDelete}
					disabled={isDeleting()}
					class={stylesCommon.action}
				>
					🗑️
				</button>
			</summary>

			<div class={styles.serverContent}>
				<div class={styles.serverInfo}>
					<table>
						<tbody>
							<tr>
								<td>Address:</td>
								<td title={props.server.address()}>
									{props.server.address()}
								</td>
							</tr>
							<tr>
								<td>Room:</td>
								<td title={props.server.room()}>
									{props.server.room()}
								</td>
							</tr>
							<tr>
								<td>Username:</td>
								<td title={props.server.username()}>
									{props.server.username()}
								</td>
							</tr>
						</tbody>
					</table>

					<A href={`/server/${props.server.uuid}/search`}>
						🔎 Search
					</A>
					<br />
					<A href={`/server/${props.server.uuid}/shares`}>
						📁 Manage Shares
					</A>
					<br />
					<A href={`/server/${props.server.uuid}/changepassword`}>
						🔑 Change Account Password
					</A>
					<Show
						when={props.server.connState() === ServerConnState.OPEN}
					>
						<br />
						<A
							href=""
							target="_self"
							onClick={doDisconnect}
							classList={{
								[stylesCommon.opacity05]:
									isPendingStateChange(),
							}}
						>
							<ConnStateIndicator
								state={ServerConnState.CLOSED}
							/>
							Disconnect
						</A>
					</Show>
					<Show
						when={
							props.server.connState() === ServerConnState.CLOSED
						}
					>
						<br />
						<A
							href=""
							target="_self"
							onClick={doConnect}
							classList={{
								[stylesCommon.opacity05]:
									isPendingStateChange(),
							}}
						>
							<ConnStateIndicator state={ServerConnState.OPEN} />
							Connect
						</A>
					</Show>
				</div>

				<div class={styles.onlineUsers}>
					<For each={props.server.onlineUsers()}>
						{(user) => (
							<OnlineUserEntry
								server={props.server}
								user={user}
							/>
						)}
					</For>
				</div>
			</div>
		</details>
	)
}

export const ServerBrowser: Component = () => {
	const state = useGlobalState()

	onMount(() => {
		state.refreshServers().catch((err) => {
			console.error('failed to refresh servers:', err)
			alert('Failed to refresh servers, see console for details')
		})
	})

	return (
		<div class={styles.container}>
			<details open={true} class={stylesCommon.sidebarContainer}>
				<summary>
					<span>Servers</span>

					<A
						title="Add New Server"
						class={stylesCommon.action}
						href="/createserver"
					>
						➕️
					</A>
				</summary>

				<For each={state.servers()}>
					{(server) => <ServerEntry server={server} />}
				</For>
			</details>
		</div>
	)
}
