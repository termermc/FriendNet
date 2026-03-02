import { Component, createSignal, For, onMount, Show } from 'solid-js'

import styles from './ServerBrowsePage.module.css'

import { useFileServerUrl, useGlobalState, useRpcClient } from '../ctx'
import { ConnectError } from '@connectrpc/connect'
import { A, useLocation, useParams } from '@solidjs/router'
import { FileMeta } from '../../pb/clientrpc/v1/rpc_pb'
import {
	makeBrowsePath,
	makeFileUrl,
	normalizePath,
	trimStrEllipsis,
} from '../util'
import { FileTable } from '../FileTable'

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

			<FileTable
				isLoading={isLoading()}
				error={error()}
				items={files().map((x) => ({ meta: x, data: undefined }))}
				parentHref={
					pathSegments.length !== 0
						? makeBrowsePath(
								uuid,
								username,
								pathSegments.slice(0, -1).join('/'),
							)
						: undefined
				}
				forItem={(item) => {
					const pth = path === '/' ? '' : path
					const filePath = pth + '/' + item.meta.name

					if (item.meta.isDir) {
						return {
							href: makeBrowsePath(uuid, username, filePath),
						}
					} else {
						const dlUrl = makeFileUrl(
							fsUrl,
							uuid,
							username,
							filePath,
							{
								download: true,
							},
						)
						const nonDlUrl = makeFileUrl(
							fsUrl,
							uuid,
							username,
							filePath,
						)

						return {
							actions: (
								<>
									<a href={nonDlUrl} target="_blank">
										🔗
									</a>
									<a href={dlUrl}>💾</a>
								</>
							),
							onClick: () => {
								state.previewFile(uuid, username, filePath)
							},
						}
					}
				}}
			/>
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
