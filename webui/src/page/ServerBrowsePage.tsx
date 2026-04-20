import { Component, createSignal, For, onMount, Show } from 'solid-js'

import styles from './ServerBrowsePage.module.css'

import { useFileServerUrl, useGlobalState, useRpcClient } from '../ctx'
import { ConnectError } from '@connectrpc/connect'
import {
	A,
	useLocation,
	useNavigate,
	useParams,
	useSearchParams,
} from '@solidjs/router'
import { FileMeta } from '../../pb/clientrpc/v1/rpc_pb'
import {
	makeBrowsePath,
	makeFileUrl,
	makeMdPreviewPath,
	normalizePath,
	trimStrEllipsis,
} from '../util'
import { FileTable } from '../FileTable'
import { QueueButton } from '../QueueButton'
import { getAutoOpenReadme } from '../uiPrefs'

const Page: Component = () => {
	const {
		uuid,
		username,
		path: pathRaw,
	} = useParams<{ uuid: string; username: string; path: string }>()

	const state = useGlobalState()
	const client = useRpcClient()
	const fsUrl = useFileServerUrl()
	const navigate = useNavigate()
	const [searchParams] = useSearchParams<{ noauto?: string }>()

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

			const shouldAutoOpen =
				searchParams.noauto !== '1' && getAutoOpenReadme()

			const stream = client.getDirFiles({
				serverUuid: server.uuid,
				username: username,
				path: path,
			})

			for await (const msg of stream) {
				if (shouldAutoOpen) {
					const readme = msg.content.find(
						(f) =>
							!f.isDir &&
							f.name.toLowerCase() === 'readme.md',
					)
					if (readme) {
						const pth = path === '/' ? '' : path
						const readmePath = pth + '/' + readme.name
						navigate(
							makeMdPreviewPath(uuid, username, readmePath),
							{ replace: true },
						)
						return
					}
				}

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
			<Show when={!error()}>
				<div class={styles.actions}>
					<QueueButton
						serverUuid={server.uuid}
						peerUsername={username}
						filePath={path}
						title="Download Directory as Zip File"
						class={styles.action}
					>
						Download Folder
					</QueueButton>
				</div>
			</Show>

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
							actions: (
								<QueueButton
									serverUuid={uuid}
									peerUsername={username}
									filePath={filePath}
									title="Download Folder"
								/>
							),
						}
					} else {
						const nonDlUrl = makeFileUrl(
							fsUrl,
							uuid,
							username,
							filePath,
						)

						const lowerName = item.meta.name.toLowerCase()
						const isMarkdown =
							lowerName.endsWith('.md') ||
							lowerName.endsWith('.markdown')

						const actions = (
							<>
								<a
									title="Open File"
									href={nonDlUrl}
									target="_blank"
								>
									🔗
								</a>
								<QueueButton
									serverUuid={uuid}
									peerUsername={username}
									filePath={filePath}
									title="Download File"
								/>
							</>
						)

						if (isMarkdown) {
							return {
								href: makeMdPreviewPath(
									uuid,
									username,
									filePath,
								),
								actions,
							}
						}

						return {
							actions,
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
