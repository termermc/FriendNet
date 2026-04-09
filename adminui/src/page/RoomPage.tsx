import styles from './RoomPage.module.css'

import {
	Accessor,
	Component,
	createSignal,
	ErrorBoundary,
	For,
	onCleanup,
	onMount,
	Setter,
	Show,
	Signal,
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
import {
	AccountInfo,
	OnlineUserInfo,
	RoomInfo,
} from '../../pb/serverrpc/v1/rpc_pb'
import stylesCommon from '../common.module.css'

const PasswordField: Component<{
	id: string
	password: Accessor<string>
	setPassword: Setter<string>
}> = (props) => {
	const [mode, setMode] = createSignal<'auto' | 'manual'>('auto')

	return (
		<div>
			<select
				id={props.id}
				onChange={(e) => {
					if (e.currentTarget.value === 'manual') {
						setMode('manual')
					} else {
						setMode('auto')
						props.setPassword('')
					}
				}}
			>
				<option value="auto">Generate</option>
				<option value="manual">Specific Password</option>
			</select>

			<Show when={mode() === 'manual'}>
				<br />
				<input
					type="password"
					id="account-password"
					value={props.password()}
					onInput={(e) => props.setPassword(e.currentTarget.value)}
					required
				/>
			</Show>
		</div>
	)
}

type AccountsProps = {
	room: RoomInfo
	accountsSignal: Signal<AccountInfo[]>
}

const CreateAccount: Component<AccountsProps> = (props) => {
	const client = useRpcClient()

	const [, setAccounts] = props.accountsSignal

	const [username, setUsername] = createSignal('')
	const [password, setPassword] = createSignal('')

	const [isCreating, setCreating] = createSignal(false)
	const [error, setError] = createSignal('')
	const [success, setSuccess] = createSignal('')

	const submit = async (e: Event) => {
		e.preventDefault()

		if (isCreating()) {
			return
		}

		const un = username().trim()
		if (!un) {
			return
		}

		try {
			setCreating(true)
			setError('')
			setSuccess('')

			const { account, generatedPassword } = await client.createAccount({
				room: props.room.name,
				username: un,
				password: password(),
			})

			if (generatedPassword == null) {
				setSuccess('Created account')
			} else {
				setSuccess(
					'Created account with password: ' + generatedPassword,
				)
			}

			setAccounts((prev) =>
				[...prev, account!].sort((a, b) =>
					a.username.localeCompare(b.username),
				),
			)

			setUsername('')
			setPassword('')
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.PermissionDenied) {
					setError(
						'The RPC method required to create accounts is not available.',
					)
					return
				}

				setError(err.message)
				return
			}

			console.error('failed to create account:', err)

			setError('Internal error, check console')
		} finally {
			setCreating(false)
		}
	}

	return (
		<div
			classList={{
				[stylesCommon.w100]: true,
				[stylesCommon.center]: true,
			}}
		>
			<Show when={error()}>
				<div class={stylesCommon.errorMessage}>{error()}</div>
			</Show>
			<Show when={success()}>
				<div class={stylesCommon.successMessage}>{success()}</div>
			</Show>

			<br />

			<form class={stylesCommon.form}>
				<table>
					<tbody>
						<tr>
							<td>
								<label for="account-username">Username</label>
							</td>
							<td>
								<input
									type="text"
									id="account-username"
									maxlength={16}
									value={username()}
									onInput={(e) =>
										setUsername(e.currentTarget.value)
									}
									required
								/>
							</td>
						</tr>
						<tr>
							<td>
								<label for="account-password">Password</label>
							</td>
							<td>
								<PasswordField
									id="account-password"
									password={password}
									setPassword={setPassword}
								/>
							</td>
						</tr>
					</tbody>
				</table>

				<input
					type="submit"
					value="Create Account"
					onClick={submit}
					disabled={isCreating()}
				/>
			</form>
		</div>
	)
}

const Account: Component<AccountsProps & { account: AccountInfo }> = (
	props,
) => {
	const client = useRpcClient()

	const [, setAccounts] = props.accountsSignal

	const acc = props.account

	const [isRemoving, setRemoving] = createSignal(false)
	const [removeError, setRemoveError] = createSignal('')
	const remove = async () => {
		if (isRemoving()) {
			return
		}

		const confirmName = prompt(
			`Are you sure? Deleting the account will kick it out of the room.\n\nType the user's name to confirm:`,
		)
		if (confirmName?.toLowerCase() !== acc.username.toLowerCase()) {
			return
		}

		try {
			setRemoving(true)
			setRemoveError('')
			await client.deleteAccount({
				room: props.room.name,
				username: acc.username,
			})
			setAccounts((prev) =>
				prev.filter((a) => a.username !== acc.username),
			)
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.PermissionDenied) {
					setRemoveError(
						'The RPC method required to delete accounts is not available.',
					)
					return
				}

				setRemoveError(err.message)
				return
			}

			console.error('failed to remove account:', err)

			setRemoveError('Internal error, check console')
		} finally {
			setRemoving(false)
		}
	}

	const [changePassError, setChangePassError] = createSignal('')
	const [isChangingPass, setChangingPass] = createSignal(false)
	const [changePassSuccess, setChangePassSuccess] = createSignal('')
	const [newPass, setNewPass] = createSignal('')
	const changePass = async (e: Event) => {
		e.preventDefault()

		if (isChangingPass()) {
			return
		}

		try {
			setChangingPass(true)
			setChangePassError('')
			setChangePassSuccess('')

			const { generatedPassword } = await client.updateAccountPassword({
				room: props.room.name,
				username: acc.username,
				password: newPass(),
			})

			if (generatedPassword == null) {
				setChangePassSuccess('Password changed')
			} else {
				setChangePassSuccess(
					'Password changed to: ' + generatedPassword,
				)
			}
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.PermissionDenied) {
					setChangePassError(
						'The RPC method required to change passwords is not available.',
					)
					return
				}

				setChangePassError(err.message)
				return
			}

			console.error('failed to change password:', err)

			setChangePassError('Internal error, check console')
		} finally {
			setChangingPass(false)
		}
	}

	return (
		<details class={styles.account}>
			<summary class={styles.accountUsername}>👤 {acc.username}</summary>
			<div class={styles.accountOptions}>
				<Show when={removeError()}>
					<div class={stylesCommon.errorMessage}>{removeError()}</div>
					<br />
				</Show>

				<button onClick={remove} disabled={isRemoving()}>
					🗑️ Delete Account
				</button>

				<details>
					<summary>Change Password</summary>

					<div>
						<br/>

						<Show when={changePassError()}>
							<div class={stylesCommon.errorMessage}>
								{changePassError()}
							</div>
							<br />
						</Show>

						<Show when={changePassSuccess()}>
							<div class={stylesCommon.successMessage}>
								{changePassSuccess()}
							</div>
							<br />
						</Show>

						<form class={stylesCommon.form} onSubmit={changePass}>
							<table>
								<tbody>
									<tr>
										<td>
											<label for="account-new-password">
												Password
											</label>
										</td>
										<td>
											<PasswordField
												id="account-new-password"
												password={newPass}
												setPassword={setNewPass}
											/>
										</td>
									</tr>
								</tbody>
							</table>

							<input
								type="submit"
								value="Change Password"
								disabled={isChangingPass()}
							/>
						</form>
					</div>
				</details>
			</div>
		</details>
	)
}

const ManageAccounts: Component<AccountsProps> = (props) => {
	const client = useRpcClient()

	const [accounts, setAccounts] = props.accountsSignal

	const initialLoad = createAsync(async () => {
		const { accounts } = await client.getAccounts({
			room: props.room.name,
		})
		setAccounts(
			accounts.sort((a, b) => a.username.localeCompare(b.username)),
		)
		return true
	})

	const [filter, setFilter] = createSignal('')

	return (
		<div class={styles.accounts}>
			<details open>
				<summary>Create Account</summary>

				<CreateAccount
					room={props.room}
					accountsSignal={props.accountsSignal}
				/>
			</details>

			<br />

			<ErrorBoundary
				fallback={(err) => {
					if (err instanceof ConnectError) {
						if (err.code === Code.PermissionDenied) {
							return (
								<div class={stylesCommon.errorMessage}>
									The RPC method required to list accounts is
									not available.
								</div>
							)
						}
					}

					console.error('failed to load accounts:', err)

					return (
						<div class={stylesCommon.errorMessage}>
							An unexpected error occurred while loading accounts.
						</div>
					)
				}}
			>
				<Suspense fallback={<i>Loading...</i>}>
					{initialLoad()}

					<Show when={accounts().length > 0}>
						<input
							type="text"
							placeholder="Filter accounts..."
							value={filter()}
							onInput={(e) => setFilter(e.target.value)}
						/>

						<For
							each={accounts().filter((x) =>
								x.username.includes(filter().trim()),
							)}
						>
							{(account) => (
								<Account
									accountsSignal={props.accountsSignal}
									room={props.room}
									account={account}
								/>
							)}
						</For>
					</Show>
				</Suspense>
			</ErrorBoundary>
		</div>
	)
}

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
			setRemoveError('')

			await client.deleteRoom({ name: room.name })

			removeRoom(room)
			navigate('/')
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.PermissionDenied) {
					setRemoveError(
						'The RPC method required to delete rooms is not available.',
					)
					return
				}

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

	const accountsSignal = createSignal<AccountInfo[]>([])

	return (
		<div class={styles.container}>
			<div class={styles.main}>
				<h1>Room: {room.name}</h1>

				<Show when={removeError()}>
					<div class={stylesCommon.errorMessage}>{removeError()}</div>

					<br />
				</Show>

				<button onClick={remove} disabled={isRemoving()}>
					🗑️ Delete Room
				</button>

				<h2>Manage Accounts</h2>
				<ManageAccounts room={room} accountsSignal={accountsSignal} />
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
										<div class={styles.onlineUser}>
											<div
												class={styles.onlineUserStatus}
											/>
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
