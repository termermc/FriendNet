import type { Component } from 'solid-js'
import {
	For,
	Show,
	createEffect,
	createMemo,
	createSignal,
	onMount,
} from 'solid-js'
import { createStore } from 'solid-js/store'
import { createClient, type Interceptor } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import {
	ClientRpcService,
	type FileMeta,
	type ServerInfo,
	type ShareInfo,
} from './protobuf'

type Notice = {
	tone: 'error' | 'info'
	message: string
}

type AppProps = {
	bearerToken: string
}

const formatBytes = (size: bigint) => {
	if (size < 1024n) {
		return `${size.toString()} B`
	}
	const units = ['KB', 'MB', 'GB', 'TB', 'PB']
	let value = size
	let index = 0
	while (value >= 1024n && index < units.length - 1) {
		value /= 1024n
		index += 1
	}
	return `${value.toString()} ${units[index]}`
}

const normalizePath = (path: string) => {
	const trimmed = path.trim()
	if (!trimmed || trimmed === '/') {
		return '/'
	}
	const withSlash = trimmed.startsWith('/') ? trimmed : `/${trimmed}`
	return withSlash.replace(/\/+$/, '').replace(/\/{2,}/g, '/')
}

const App: Component<AppProps> = (props) => {
	const authInterceptor: Interceptor = (next) => async (req) => {
		req.header.set('Authorization', `Bearer ${props.bearerToken}`)
		return next(req)
	}

	const client = createClient(
		ClientRpcService,
		createConnectTransport({
			baseUrl: 'http://127.0.0.1:20039',
			useBinaryFormat: true,
			interceptors: [authInterceptor],
		}),
	)

	const [notice, setNotice] = createSignal<Notice | null>(null)
	const [servers, setServers] = createSignal<ServerInfo[]>([])
	const [serversLoading, setServersLoading] = createSignal(false)
	const [sharesByServer, setSharesByServer] = createSignal<
		Record<string, ShareInfo[]>
	>({})
	const [sharesLoading, setSharesLoading] = createSignal<Record<string, boolean>>(
		{},
	)
	const [usersByServer, setUsersByServer] = createSignal<
		Record<string, string[]>
	>({})
	const [selectedServerId, setSelectedServerId] = createSignal<string | null>(
		null,
	)
	const [selectedUser, setSelectedUser] = createSignal('')
	const [selectedShareName, setSelectedShareName] = createSignal<string | null>(
		null,
	)
	const [dirEntries, setDirEntries] = createSignal<FileMeta[]>([])
	const [dirPath, setDirPath] = createSignal('/')
	const [dirLoading, setDirLoading] = createSignal(false)
	const [serverForm, setServerForm] = createStore({
		name: '',
		address: '',
		room: '',
		username: '',
		password: '',
	})
	const [shareForm, setShareForm] = createStore({
		serverUuid: '',
		name: '',
		path: '',
	})
	const [userDrafts, setUserDrafts] = createStore<Record<string, string>>({})

	let dirLoadToken = 0

	const selectedServer = createMemo(
		() =>
			servers().find((server) => server.uuid === selectedServerId()) ?? null,
	)
	const selectedServerShares = createMemo(() => {
		const id = selectedServerId()
		return id ? sharesByServer()[id] ?? [] : []
	})
	const sortedDirEntries = createMemo(() => {
		const list = [...dirEntries()]
		list.sort((a, b) => {
			if (a.isDir !== b.isDir) {
				return a.isDir ? -1 : 1
			}
			return a.name.localeCompare(b.name)
		})
		return list
	})
	const serverFormValid = createMemo(
		() =>
			serverForm.name.trim() &&
			serverForm.address.trim() &&
			serverForm.room.trim() &&
			serverForm.username.trim(),
	)
	const shareFormValid = createMemo(
		() =>
			shareForm.serverUuid.trim() &&
			shareForm.name.trim() &&
			shareForm.path.trim(),
	)
	const breadcrumbs = createMemo(() => {
		const path = normalizePath(dirPath())
		const parts = path.split('/').filter(Boolean)
		const crumbs: { label: string; path: string }[] = [
			{ label: 'Root', path: '/' },
		]
		let current = ''
		for (const part of parts) {
			current += `/${part}`
			crumbs.push({ label: part, path: current })
		}
		return crumbs
	})

	const showNotice = (tone: Notice['tone'], message: string) => {
		setNotice({ tone, message })
	}

	const loadShares = async (serverUuid: string) => {
		setSharesLoading((prev) => ({ ...prev, [serverUuid]: true }))
		try {
			const resp = await client.getShares({ serverUuid })
			setSharesByServer((prev) => ({
				...prev,
				[serverUuid]: resp.shares ?? [],
			}))
		} catch (err) {
			showNotice('error', `Failed to load shares: ${String(err)}`)
		} finally {
			setSharesLoading((prev) => ({ ...prev, [serverUuid]: false }))
		}
	}

	const loadServers = async () => {
		setServersLoading(true)
		setNotice(null)
		try {
			const resp = await client.getServers({})
			const list = resp.servers ?? []
			setServers(list)
			await Promise.all(list.map((server) => loadShares(server.uuid)))
		} catch (err) {
			showNotice('error', `Failed to load servers: ${String(err)}`)
		} finally {
			setServersLoading(false)
		}
	}

	const createServer = async (event: Event) => {
		event.preventDefault()
		if (!serverFormValid()) {
			showNotice('error', 'Fill in all required server fields.')
			return
		}
		setNotice(null)
		try {
			await client.createServer({
				name: serverForm.name.trim(),
				address: serverForm.address.trim(),
				room: serverForm.room.trim(),
				username: serverForm.username.trim(),
				password: serverForm.password.trim(),
			})
			setServerForm({
				name: '',
				address: '',
				room: '',
				username: '',
				password: '',
			})
			await loadServers()
			showNotice('info', 'Server created and connected.')
		} catch (err) {
			showNotice('error', `Failed to create server: ${String(err)}`)
		}
	}

	const deleteServer = async (serverUuid: string) => {
		try {
			await client.deleteServer({ uuid: serverUuid })
			await loadServers()
			showNotice('info', 'Server deleted.')
		} catch (err) {
			showNotice('error', `Failed to delete server: ${String(err)}`)
		}
	}

	const connectServer = async (serverUuid: string) => {
		try {
			await client.connectServer({ uuid: serverUuid })
			showNotice('info', 'Server connected.')
		} catch (err) {
			showNotice('error', `Failed to connect server: ${String(err)}`)
		}
	}

	const disconnectServer = async (serverUuid: string) => {
		try {
			await client.disconnectServer({ uuid: serverUuid })
			showNotice('info', 'Server disconnected.')
		} catch (err) {
			showNotice('error', `Failed to disconnect server: ${String(err)}`)
		}
	}

	const createShare = async (event: Event) => {
		event.preventDefault()
		if (!shareFormValid()) {
			showNotice('error', 'Fill in all required share fields.')
			return
		}
		setNotice(null)
		try {
			await client.createShare({
				serverUuid: shareForm.serverUuid,
				name: shareForm.name.trim(),
				path: shareForm.path.trim(),
			})
			setShareForm({
				serverUuid: shareForm.serverUuid,
				name: '',
				path: '',
			})
			await loadShares(shareForm.serverUuid)
			showNotice('info', 'Share created.')
		} catch (err) {
			showNotice('error', `Failed to create share: ${String(err)}`)
		}
	}

	const deleteShare = async (serverUuid: string, shareName: string) => {
		try {
			await client.deleteShare({ serverUuid, name: shareName })
			await loadShares(serverUuid)
			showNotice('info', 'Share deleted.')
		} catch (err) {
			showNotice('error', `Failed to delete share: ${String(err)}`)
		}
	}

	const addOnlineUser = (serverUuid: string) => {
		const draft = userDrafts[serverUuid]?.trim() ?? ''
		if (!draft) {
			return
		}
		setUsersByServer((prev) => {
			const current = prev[serverUuid] ?? []
			if (current.includes(draft)) {
				return prev
			}
			return { ...prev, [serverUuid]: [...current, draft] }
		})
		setUserDrafts(serverUuid, '')
	}

	const selectUser = (serverUuid: string, username: string) => {
		setSelectedServerId(serverUuid)
		setSelectedUser(username)
		setSelectedShareName(null)
		setDirPath('/')
		setDirEntries([])
	}

	const selectShare = (serverUuid: string, share: ShareInfo) => {
		setSelectedServerId(serverUuid)
		setSelectedShareName(share.name)
		setDirPath(normalizePath(share.path))
		setDirEntries([])
	}

	const loadDirectory = async () => {
		const serverUuid = selectedServerId()
		const username = selectedUser().trim()
		const path = normalizePath(dirPath())
		if (!serverUuid) {
			showNotice('error', 'Select a server before browsing files.')
			return
		}
		if (!username) {
			showNotice('error', 'Enter or pick a username to browse.')
			return
		}
		setDirEntries([])
		setDirLoading(true)
		const token = (dirLoadToken += 1)
		try {
			const stream = client.getDirFiles({
				serverUuid,
				username,
				path,
			})
			for await (const msg of stream) {
				if (token !== dirLoadToken) {
					break
				}
				if (msg.content?.length) {
					setDirEntries((prev) => [...prev, ...msg.content])
				}
			}
		} catch (err) {
			showNotice('error', `Failed to load directory: ${String(err)}`)
		} finally {
			if (token === dirLoadToken) {
				setDirLoading(false)
			}
		}
	}

	const goUp = () => {
		const path = normalizePath(dirPath())
		if (path === '/') {
			return
		}
		const parts = path.split('/').filter(Boolean)
		parts.pop()
		setDirPath(parts.length ? `/${parts.join('/')}` : '/')
	}

	const enterDirectory = (entry: FileMeta) => {
		if (!entry.isDir) {
			return
		}
		const path = normalizePath(dirPath())
		const next = path === '/' ? `/${entry.name}` : `${path}/${entry.name}`
		setDirPath(normalizePath(next))
		void loadDirectory()
	}

	createEffect(() => {
		const list = servers()
		const current = selectedServerId()
		if (!list.length) {
			setSelectedServerId(null)
			return
		}
		if (!current || !list.some((server) => server.uuid === current)) {
			setSelectedServerId(list[0].uuid)
		}
	})

	createEffect(() => {
		const server = selectedServer()
		if (server && !selectedUser()) {
			setSelectedUser(server.username)
		}
	})

	createEffect(() => {
		const id = selectedServerId()
		if (id) {
			setShareForm('serverUuid', id)
		}
	})

	onMount(() => {
		void loadServers()
	})

	return (
		<div class="app">
			<header class="topbar">
				<div class="brand">
					<div class="brand-title">FriendNet</div>
					<div class="brand-subtitle">Client Web Console</div>
				</div>
				<div class="top-actions">
					<button class="ghost" onClick={() => void loadServers()}>
						Refresh
					</button>
					<div class="token-pill">Bearer token loaded</div>
				</div>
			</header>

			<div class="layout">
				<aside class="sidebar">
					<div class="sidebar-header">
						<div>
							<h2>Servers</h2>
							<p>Manage connections, shares, and online users.</p>
						</div>
						<Show when={serversLoading()}>
							<span class="pill">Loading…</span>
						</Show>
					</div>

					<div class="server-list">
						<Show
							when={servers().length > 0}
							fallback={<div class="empty">No servers configured.</div>}
						>
							<For each={servers()}>
								{(server) => (
									<details
										open={server.uuid === selectedServerId()}
										class="server-card"
									>
										<summary
											class="server-summary"
											onClick={() => setSelectedServerId(server.uuid)}
										>
											<div>
												<div class="server-name">{server.name}</div>
												<div class="server-meta">
													<span>{server.address}</span>
													<span>Room {server.room}</span>
													<span>User {server.username}</span>
												</div>
											</div>
											<div class="server-actions">
												<button
													class="ghost"
													onClick={(event) => {
														event.preventDefault()
														void connectServer(server.uuid)
													}}
												>
													Connect
												</button>
												<button
													class="ghost"
													onClick={(event) => {
														event.preventDefault()
														void disconnectServer(server.uuid)
													}}
												>
													Disconnect
												</button>
												<button
													class="danger"
													onClick={(event) => {
														event.preventDefault()
														void deleteServer(server.uuid)
													}}
												>
													Delete
												</button>
											</div>
										</summary>

										<div class="server-section">
											<div class="section-header">
												<span>Shares</span>
												<button
													class="ghost"
													onClick={() => void loadShares(server.uuid)}
												>
													Reload
												</button>
												<Show when={sharesLoading()[server.uuid]}>
													<span class="pill">Loading…</span>
												</Show>
											</div>
											<div class="list">
												<Show
													when={(sharesByServer()[server.uuid] ?? []).length}
													fallback={
														<div class="empty">No shares yet.</div>
													}
												>
													<For each={sharesByServer()[server.uuid] ?? []}>
														{(share) => (
															<div
																class={`list-row ${
																	selectedShareName() === share.name &&
																	selectedServerId() === server.uuid
																		? 'active'
																		: ''
																}`}
																onClick={() => selectShare(server.uuid, share)}
															>
																<div>
																	<div class="list-title">{share.name}</div>
																	<div class="list-subtitle">
																		{share.path}
																	</div>
																</div>
																<button
																	class="ghost"
																	onClick={(event) => {
																		event.stopPropagation()
																		void deleteShare(server.uuid, share.name)
																	}}
																>
																	Delete
																</button>
															</div>
														)}
													</For>
												</Show>
											</div>
										</div>

										<div class="server-section">
											<div class="section-header">
												<span>Online Users</span>
											</div>
											<p class="section-note">
												Online user listing is not exposed in client RPC yet.
												Add usernames manually to browse.
											</p>
											<div class="user-add">
												<input
													type="text"
													placeholder="username"
													value={userDrafts[server.uuid] ?? ''}
													onInput={(event) =>
														setUserDrafts(server.uuid, event.currentTarget.value)
													}
												/>
												<button
													class="ghost"
													onClick={() => addOnlineUser(server.uuid)}
												>
													Add
												</button>
											</div>
											<div class="list">
												<Show
													when={(usersByServer()[server.uuid] ?? []).length}
													fallback={
														<div class="empty">No users listed.</div>
													}
												>
													<For each={usersByServer()[server.uuid] ?? []}>
														{(user) => (
															<button
																class={`list-row ${
																	selectedUser() === user &&
																	selectedServerId() === server.uuid
																		? 'active'
																		: ''
																}`}
																onClick={() => selectUser(server.uuid, user)}
															>
																<div>
																	<div class="list-title">{user}</div>
																	<div class="list-subtitle">
																		Browse shared files
																	</div>
																</div>
															</button>
														)}
													</For>
												</Show>
											</div>
										</div>
									</details>
								)}
							</For>
						</Show>
					</div>

					<div class="panel">
						<h3>Create Server</h3>
						<form class="form" onSubmit={createServer}>
							<label>
								Name*
								<input
									type="text"
									value={serverForm.name}
									onInput={(event) =>
										setServerForm('name', event.currentTarget.value)
									}
								/>
							</label>
							<label>
								Address*
								<input
									type="text"
									placeholder="server:port"
									value={serverForm.address}
									onInput={(event) =>
										setServerForm('address', event.currentTarget.value)
									}
								/>
							</label>
							<label>
								Room*
								<input
									type="text"
									value={serverForm.room}
									onInput={(event) =>
										setServerForm('room', event.currentTarget.value)
									}
								/>
							</label>
							<label>
								Username*
								<input
									type="text"
									value={serverForm.username}
									onInput={(event) =>
										setServerForm('username', event.currentTarget.value)
									}
								/>
							</label>
							<label>
								Password
								<input
									type="password"
									value={serverForm.password}
									onInput={(event) =>
										setServerForm('password', event.currentTarget.value)
									}
								/>
							</label>
							<button type="submit" class="primary">
								Create server
							</button>
						</form>
					</div>

					<div class="panel">
						<h3>Create Share</h3>
						<form class="form" onSubmit={createShare}>
							<label>
								Server
								<select
									value={shareForm.serverUuid}
									onChange={(event) =>
										setShareForm('serverUuid', event.currentTarget.value)
									}
								>
									<option value="" disabled>
										Select server
									</option>
									<For each={servers()}>
										{(server) => (
											<option value={server.uuid}>{server.name}</option>
										)}
									</For>
								</select>
							</label>
							<label>
								Share name*
								<input
									type="text"
									value={shareForm.name}
									onInput={(event) =>
										setShareForm('name', event.currentTarget.value)
									}
								/>
							</label>
							<label>
								Path*
								<input
									type="text"
									placeholder="/Users/alex/Music"
									value={shareForm.path}
									onInput={(event) =>
										setShareForm('path', event.currentTarget.value)
									}
								/>
							</label>
							<button type="submit" class="primary">
								Create share
							</button>
						</form>
					</div>
				</aside>

				<main class="main">
					<Show when={notice()}>
						{(alert) => (
							<div class={`notice ${alert().tone}`}>
								{alert().message}
							</div>
						)}
					</Show>

					<section class="panel browser">
						<div class="browser-head">
							<div>
								<h2>File Browser</h2>
								<p>
									Browse shared directories. Download is not implemented yet.
								</p>
							</div>
							<div class="context">
								<span>
									Server:{' '}
									{selectedServer()?.name ?? 'None'}
								</span>
								<span>User: {selectedUser() || '—'}</span>
								<span>
									Share:{' '}
									{selectedShareName() ?? 'None'}
								</span>
							</div>
						</div>

						<div class="browser-controls">
							<label>
								Username
								<input
									type="text"
									placeholder="friend_username"
									value={selectedUser()}
									onInput={(event) =>
										setSelectedUser(event.currentTarget.value)
									}
								/>
							</label>
							<label>
								Path
								<input
									type="text"
									value={dirPath()}
									onInput={(event) =>
										setDirPath(event.currentTarget.value)
									}
								/>
							</label>
							<div class="browser-buttons">
								<button class="primary" onClick={() => void loadDirectory()}>
									Load directory
								</button>
								<button class="ghost" onClick={goUp}>
									Up one level
								</button>
							</div>
						</div>

						<div class="breadcrumbs">
							<For each={breadcrumbs()}>
								{(crumb) => (
									<button
										class="crumb"
										onClick={() => setDirPath(crumb.path)}
									>
										{crumb.label}
									</button>
								)}
							</For>
						</div>

						<div class="browser-list">
							<Show when={dirLoading()}>
								<div class="empty">Loading directory…</div>
							</Show>
							<Show when={!dirLoading() && sortedDirEntries().length === 0}>
								<div class="empty">No files found.</div>
							</Show>
							<For each={sortedDirEntries()}>
								{(entry) => (
									<div class="file-row">
										<button
											class="file-name"
											onClick={() => enterDirectory(entry)}
										>
											<span
												class={`dot ${entry.isDir ? 'dir' : 'file'}`}
											></span>
											{entry.name}
										</button>
										<div class="file-meta">
											{entry.isDir ? 'Folder' : formatBytes(entry.size)}
										</div>
										<button class="ghost" disabled>
											Download
										</button>
									</div>
								)}
							</For>
						</div>
					</section>
				</main>
			</div>
		</div>
	)
}

export default App
