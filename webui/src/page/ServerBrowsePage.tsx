import { Component, createSignal, For, onMount, Show } from 'solid-js'

import styles from './ServerBrowsePage.module.css'
import stylesCommon from '../common.module.css'

import { useFileServerUrl, useGlobalState, useRpcClient } from '../ctx'
import { ConnectError } from '@connectrpc/connect'
import { A, useLocation, useParams } from '@solidjs/router'
import { FileMeta } from '../../pb/clientrpc/v1/rpc_pb'
import {
	guessFileCategory,
	makeBrowsePath,
	makeFileUrl,
	normalizePath,
	trimStrEllipsis,
} from '../util'

const Page: Component = () => {
	const {
		uuid,
		username,
		path: pathRaw,
	} = useParams<{ uuid: string; username: string; path: string }>()

	const state = useGlobalState()
	const client = useRpcClient()
	const fsUrl = useFileServerUrl()

	const server = state.getServerByUuid(uuid)
	if (!server) {
		return <h1>No such server "{uuid}"</h1>
	}

	// Normalize path.
	const { path, segments: pathSegments } = normalizePath(
		decodeURIComponent(pathRaw),
	)

	const [files, setFiles] = createSignal<FileMeta[]>([])
	const [isLoading, setLoading] = createSignal(false)
	const [error, setError] = createSignal('')

	onMount(async () => {
		try {
			setLoading(true)

			const stream = client.getDirFiles({
				serverUuid: server.uuid,
				username: username,
				path: path,
			})

			for await (const msg of stream) {
				const res = [...files(), ...msg.content]
				res.sort((a, b) => {
					if (a.isDir && !b.isDir) {
						return -1
					}
					if (!a.isDir && b.isDir) {
						return 1
					}

					return a.name.localeCompare(b.name)
				})

				setFiles(res)
			}
		} catch (err) {
			if (err instanceof ConnectError) {
				setError(err.message)
			} else {
				console.error('failed to browse path:', err)
				setError('Internal error, check console')
			}
		} finally {
			setLoading(false)
		}
	})

	return (
		<div class={styles.container}>
			<div class={styles.location}>
				<div class={styles.segment}>🖧 {server.name()}</div>
				<A
					href={makeBrowsePath(uuid, username, '')}
					class={styles.segment}
				>
					👤 {username}
				</A>

				<For each={pathSegments}>
					{(seg, i) => (
						<A
							title={seg}
							href={makeBrowsePath(
								uuid,
								username,
								pathSegments.slice(0, i() + 1).join('/'),
							)}
							classList={{
								[styles.segment]: true,
								[styles.error]:
									i() === pathSegments.length - 1 &&
									error() !== '',
							}}
						>
							{trimStrEllipsis(seg, 20)}
						</A>
					)}
				</For>
			</div>

			<div class={styles.files}>
				<table>
					<thead>
						<tr>
							<th>File</th>
							<th>Actions</th>
						</tr>
					</thead>
					<tbody>
						<Show when={isLoading()}>
							<tr>
								<td colspan="2">Loading...</td>
							</tr>
						</Show>
						<Show when={error()}>
							<tr>
								<td
									colspan="2"
									class={stylesCommon.errorMessage}
								>
									{error()}
								</td>
							</tr>
						</Show>

						<Show when={pathSegments.length !== 0}>
							<tr>
								<td>
									<A
										href={makeBrowsePath(
											uuid,
											username,
											pathSegments.slice(0, -1).join('/'),
										)}
										title="Up a directory"
										classList={{
											[stylesCommon.w100]: true,
											[stylesCommon.displayInlineBlock]: true,
										}}
									>
										▲ ..
									</A>
								</td>
							</tr>
						</Show>
						<For each={files()}>
							{(meta) => {
								let pth = path === '/' ? '' : path

								const filePath = pth + '/' + meta.name
								const dlUrl = makeFileUrl(
									fsUrl,
									uuid,
									username,
									filePath,
									true,
								)
								const nonDlUrl = makeFileUrl(
									fsUrl,
									uuid,
									username,
									filePath,
									false,
								)

								let emoji: string
								if (meta.isDir) {
									emoji = '📁'
								} else {
									const cat = guessFileCategory(meta.name)
									switch (cat) {
										case 'text':
											emoji = '📜'
											break
										case 'image':
											emoji = '🖼️'
											break
										case 'video':
											emoji = '🎞️'
											break
										case 'audio':
											emoji = '🎵'
											break
										case 'other':
											emoji = '📄'
											break
									}
								}

								const label = trimStrEllipsis(
									emoji + ' ' + meta.name,
									100,
								)

								return (
									<tr>
										<Show
											when={meta.isDir}
											fallback={
												<td
													title={meta.name}
													onClick={() =>
														state.previewFile(
															uuid,
															username,
															filePath,
														)
													}
													class={styles.label}
												>
													<span>{label}</span>
												</td>
											}
										>
											<td
												title={meta.name}
												class={styles.label}
											>
												<A
													href={makeBrowsePath(
														uuid,
														username,
														filePath,
													)}
												>
													{label}
												</A>
											</td>
										</Show>
										<td class={styles.actionsTd}>
											<div class={styles.actions}>
												<Show when={!meta.isDir}>
													<a
														href={nonDlUrl}
														target="_blank"
													>
														🔗
													</a>
													<a href={dlUrl}>💾</a>
												</Show>
											</div>
										</td>
									</tr>
								)
							}}
						</For>
					</tbody>
				</table>
			</div>
		</div>
	)
}

export const ServerBrowsePage: Component = () => {
	const loc = useLocation()

	return (
		<Show when={loc.pathname} keyed>
			<Page />
		</Show>
	)
}
