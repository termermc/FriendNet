import { Component, createSignal, For, onCleanup, onMount } from 'solid-js'
import { useGlobalState, useRpcClient } from '../ctx'

import styles from './ServerBrowser.module.css'
import stylesCommon from '../common.module.css'

import { OnlineUser, Server } from '../state'
import { A } from '@solidjs/router'
import { Code, ConnectError } from '@connectrpc/connect'
import { makeBrowsePath } from '../util'

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
	const client = useRpcClient()

	const refreshUsers = () => {
		props.server.refreshOnlineUsers(client).catch((err) => {
			if (err instanceof ConnectError && err.code === Code.NotFound) {
				// Server was probably deleted.
				state.refreshServers(client).catch((err) => {
					console.error(
						'failed to refresh servers after apparently server deletion:',
						err,
					)
				})
				return
			}

			console.error('failed to refresh online users:', err)
			alert('Failed to refresh online users, see console for details')
		})
	}

	onMount(() => {
		refreshUsers()
	})

	const userRefresher = setInterval(() => {
		refreshUsers()
	}, 5_000)
	onCleanup(() => clearInterval(userRefresher))

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
			await state.deleteServer(client, props.server.uuid)
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

	return (
		<details
			open={true}
			classList={{
				[styles.server]: true,
				[stylesCommon.sidebarContainer]: true,
			}}
		>
			<summary>
				<span title={props.server.name()}>{props.server.name()}</span>

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

					<A href={`/server/${props.server.uuid}/shares`}>
						📁 Manage Shares
					</A>
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
	const client = useRpcClient()

	onMount(() => {
		state.refreshServers(client).catch((err) => {
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
						title="Create New Server"
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
