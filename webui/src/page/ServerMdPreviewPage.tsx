import styles from './ServerMdPreviewPage.module.css'
import stylesCommon from '../common.module.css'

import {
	Component,
	createEffect,
	createResource,
	createSignal,
	For,
	Match,
	Show,
	Switch,
} from 'solid-js'
import { A, useLocation, useNavigate, useParams } from '@solidjs/router'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

import { useFileServerUrl, useGlobalState } from '../ctx'
import {
	makeBrowsePath,
	makeFileUrl,
	makeMdPreviewPath,
	normalizePath,
	trimStrEllipsis,
} from '../util'
import { QueueButton } from '../QueueButton'

const MAX_SIZE_BYTES = 1024 * 1024 // 1 MiB

const PURIFY_CONFIG = {
	ALLOWED_TAGS: [
		'p',
		'h1',
		'h2',
		'h3',
		'h4',
		'h5',
		'h6',
		'ul',
		'ol',
		'li',
		'code',
		'pre',
		'strong',
		'em',
		's',
		'del',
		'blockquote',
		'table',
		'thead',
		'tbody',
		'tr',
		'th',
		'td',
		'a',
		'img',
		'hr',
		'br',
		'div',
		'span',
		'input',
	],
	ALLOWED_ATTR: [
		'href',
		'target',
		'rel',
		'src',
		'alt',
		'title',
		'class',
		'id',
		'checked',
		'disabled',
		'type',
		'colspan',
		'rowspan',
	],
	FORBID_TAGS: [
		'script',
		'iframe',
		'object',
		'embed',
		'style',
		'meta',
		'form',
		'button',
	],
	ALLOW_DATA_ATTR: false,
}

type FetchResult =
	| { kind: 'ok'; markdown: string }
	| { kind: 'too-large'; size: number }
	| { kind: 'error'; message: string }

async function fetchMarkdown(url: string): Promise<FetchResult> {
	try {
		const head = await fetch(url, { method: 'HEAD' })
		if (head.status !== 200) {
			return {
				kind: 'error',
				message: `server returned status ${head.status}`,
			}
		}

		const lenHeader = head.headers.get('Content-Length')
		if (lenHeader !== null) {
			const size = Number.parseInt(lenHeader, 10)
			if (Number.isFinite(size) && size > MAX_SIZE_BYTES) {
				return { kind: 'too-large', size }
			}
		}

		const res = await fetch(url)
		if (res.status !== 200) {
			return {
				kind: 'error',
				message: `server returned status ${res.status}`,
			}
		}

		return { kind: 'ok', markdown: await res.text() }
	} catch (err) {
		console.error('failed to fetch markdown:', url, err)
		return {
			kind: 'error',
			message: 'failed to fetch, check browser console',
		}
	}
}

type RewriteDeps = {
	serverUuid: string
	username: string
	shareName: string
	dir: string
	fsUrl: string
	onMdLinkClick: (resolvedPath: string, ev: MouseEvent) => void
}

function rewriteDom(root: HTMLElement, deps: RewriteDeps) {
	const { serverUuid, username, shareName, dir, fsUrl, onMdLinkClick } = deps

	const inShare = (segments: string[]) =>
		segments.length > 0 && segments[0] === shareName

	const blockLink = (link: HTMLAnchorElement, reason: string) => {
		link.setAttribute('href', '#')
		link.setAttribute('data-blocked', 'true')
		link.setAttribute('title', reason)
	}

	const blockImg = (img: HTMLImageElement, reason: string) => {
		img.removeAttribute('src')
		img.setAttribute('data-blocked', 'true')
		img.setAttribute('alt', img.getAttribute('alt') ?? '[blocked image]')
		img.setAttribute('title', reason)
	}

	for (const link of Array.from(root.querySelectorAll('a'))) {
		const href = link.getAttribute('href')
		if (!href) {
			continue
		}

		if (href.startsWith('#')) {
			continue
		}

		if (/^https?:\/\//i.test(href)) {
			link.setAttribute('target', '_blank')
			link.setAttribute('rel', 'noopener noreferrer')
			if (!link.hasAttribute('title')) {
				link.setAttribute(
					'title',
					'Middle-click or right-click to open',
				)
			}
			link.addEventListener('click', (ev) => {
				if (ev.button !== 0) return
				if (ev.ctrlKey || ev.metaKey || ev.shiftKey || ev.altKey) {
					return
				}
				ev.preventDefault()
			})
			continue
		}

		if (/^[a-z][a-z0-9+.-]*:/i.test(href)) {
			blockLink(link, 'only http(s) external links are allowed')
			continue
		}

		if (href.startsWith('/') || href.includes('\\')) {
			blockLink(link, 'absolute paths are not allowed')
			continue
		}

		const resolved = normalizePath(dir + '/' + href)
		if (!inShare(resolved.segments)) {
			blockLink(link, 'link target escapes the current share')
			continue
		}

		const lower = href.toLowerCase()
		const last = resolved.segments[resolved.segments.length - 1] ?? ''
		const lastLower = last.toLowerCase()
		const isMd =
			lastLower.endsWith('.md') ||
			lastLower.endsWith('.markdown') ||
			lower.endsWith('.md') ||
			lower.endsWith('.markdown')

		if (isMd) {
			const mdHref = makeMdPreviewPath(serverUuid, username, resolved.path)
			link.setAttribute('href', mdHref)
			link.addEventListener('click', (ev) =>
				onMdLinkClick(resolved.path, ev),
			)
			continue
		}

		link.setAttribute(
			'href',
			makeFileUrl(fsUrl, serverUuid, username, resolved.path, {
				allowCache: true,
			}),
		)
		link.setAttribute('target', '_blank')
		link.setAttribute('rel', 'noopener noreferrer')
	}

	for (const img of Array.from(root.querySelectorAll('img'))) {
		const src = img.getAttribute('src')
		if (!src) {
			continue
		}

		if (src.startsWith('data:image/')) {
			continue
		}

		if (/^[a-z][a-z0-9+.-]*:/i.test(src)) {
			blockImg(img, 'external images are not allowed')
			continue
		}

		if (src.startsWith('/') || src.includes('\\')) {
			blockImg(img, 'absolute paths are not allowed')
			continue
		}

		const resolved = normalizePath(dir + '/' + src)
		if (!inShare(resolved.segments)) {
			blockImg(img, 'image source escapes the current share')
			continue
		}

		img.setAttribute(
			'src',
			makeFileUrl(fsUrl, serverUuid, username, resolved.path, {
				allowCache: true,
			}),
		)
	}
}

const Page: Component = () => {
	const {
		uuid,
		username,
		path: pathRaw,
	} = useParams<{ uuid: string; username: string; path: string }>()

	const state = useGlobalState()
	const fsUrl = useFileServerUrl()
	const navigate = useNavigate()

	const server = state.getServerByUuid(uuid)
	if (!server) {
		return <h1>No such server "{uuid}"</h1>
	}

	const { path, segments: pathSegments } = normalizePath(
		decodeURIComponent(pathRaw),
	)

	if (pathSegments.length === 0) {
		return (
			<div class={styles.container}>
				<div class={stylesCommon.errorMessage}>
					No file specified.
				</div>
			</div>
		)
	}

	const shareName = pathSegments[0]
	const dir =
		pathSegments.length > 1
			? '/' + pathSegments.slice(0, -1).join('/')
			: '/'

	const url = makeFileUrl(fsUrl, uuid, username, path)

	const [viewAsRaw, setViewAsRaw] = createSignal(false)
	const [result] = createResource(() => url, fetchMarkdown)

	let contentRef: HTMLDivElement | undefined

	createEffect(() => {
		const res = result()
		if (!contentRef) {
			return
		}
		if (!res || res.kind !== 'ok') {
			contentRef.replaceChildren()
			return
		}
		if (viewAsRaw()) {
			return
		}

		const html = marked.parse(res.markdown, { gfm: true }) as string
		const clean = DOMPurify.sanitize(html, PURIFY_CONFIG) as string

		const scratch = document.createElement('div')
		scratch.innerHTML = clean

		rewriteDom(scratch, {
			serverUuid: uuid,
			username,
			shareName,
			dir,
			fsUrl,
			onMdLinkClick: (resolvedPath, ev) => {
				ev.preventDefault()
				navigate(makeMdPreviewPath(uuid, username, resolvedPath))
			},
		})

		contentRef.replaceChildren(...Array.from(scratch.childNodes))
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
					{(seg, i) => {
						const isLast = i() === pathSegments.length - 1
						if (isLast) {
							return (
								<span title={seg} class={styles.segment}>
									📄 {trimStrEllipsis(seg, 30)}
								</span>
							)
						}
						return (
							<A
								title={seg}
								href={makeBrowsePath(
									uuid,
									username,
									pathSegments.slice(0, i() + 1).join('/'),
								)}
								class={styles.segment}
							>
								{trimStrEllipsis(seg, 20)}
							</A>
						)
					}}
				</For>
			</div>

			<div class={styles.actions}>
				<button
					type="button"
					class={styles.closeButton}
					title="Close preview and show the directory listing"
					onClick={() => {
						navigate(
							makeBrowsePath(uuid, username, dir) +
								'?noauto=1',
							{ replace: true },
						)
					}}
				>
					✖ Close preview
				</button>
			</div>

			<Switch
				fallback={
					<div class={styles.body}>
						<Show
							when={viewAsRaw()}
							fallback={
								<div class={styles.content} ref={contentRef} />
							}
						>
							<pre class={styles.raw}>
								{result()?.kind === 'ok'
									? (result() as { markdown: string })
											.markdown
									: ''}
							</pre>
						</Show>

						<div class={styles.footer}>
							<Show
								when={viewAsRaw()}
								fallback={
									<a
										href=""
										onClick={(e) => {
											e.preventDefault()
											setViewAsRaw(true)
										}}
									>
										📜 View raw
									</a>
								}
							>
								<a
									href=""
									onClick={(e) => {
										e.preventDefault()
										setViewAsRaw(false)
									}}
								>
									👁️ View rendered
								</a>
							</Show>
						</div>
					</div>
				}
			>
				<Match when={result.loading}>
					<div class={styles.body}>
						<i>Loading...</i>
					</div>
				</Match>
				<Match when={result()?.kind === 'error'}>
					<div class={styles.body}>
						<div class={stylesCommon.errorMessage}>
							{(result() as { message: string }).message}
						</div>
					</div>
				</Match>
				<Match when={result()?.kind === 'too-large'}>
					<div class={styles.body}>
						<p>
							File too large to preview (
							{(result() as { size: number }).size} bytes, limit{' '}
							{MAX_SIZE_BYTES} bytes).
						</p>
						<p>
							<QueueButton
								serverUuid={uuid}
								peerUsername={username}
								filePath={path}
							>
								Download
							</QueueButton>
						</p>
						<p>
							<a href={url} target="_blank">
								🔗 Open in Browser
							</a>
						</p>
					</div>
				</Match>
			</Switch>
		</div>
	)
}

export const ServerMdPreviewPage: Component = () => {
	const loc = useLocation()
	return (
		<Show when={loc.pathname} keyed>
			<Page />
		</Show>
	)
}
