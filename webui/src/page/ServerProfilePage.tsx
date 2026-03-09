import styles from './ServerProfilePage.module.css'

import { Component, For, Show, Suspense } from 'solid-js'
import { useFileServerUrl, useGlobalState } from '../ctx'
import { createAsync, useLocation, useParams } from '@solidjs/router'
import { makeFileUrl } from '../util'

const Page: Component = () => {
	const { uuid, username } = useParams<{ uuid: string; username: string }>()
	const state = useGlobalState()
	const fsUrl = useFileServerUrl()

	const server = state.getServerByUuid(uuid)
	if (!server) {
		return <h1>No such server "{uuid}"</h1>
	}

	const indexUrl = makeFileUrl(fsUrl, uuid, username, '/_profile/index.html')

	const resolved = createAsync(async () => {
		try {
			const fetchRes = await fetch(indexUrl)

			if (fetchRes.status === 404) {
				if (username == server.username()) {
					return (
						<div>
							<p>You have no profile.</p>
							<p>
								To make one, create a share named{' '}
								<code>_profile</code> and put an{' '}
								<code>index.html</code> file in it.
							</p>
						</div>
					)
				}

				return <i>This user has no profile.</i>
			}

			if (fetchRes.status !== 200) {
				let text = ''
				try {
					text = await fetchRes.text()
				} catch (err) {}

				return (
					<div>
						<p>Server returned status {fetchRes.status}</p>
						<pre>{text}</pre>
					</div>
				)
			}

			const rawHtml = await fetchRes.text()

			// Before displaying the iframe, check for problematic elements.
			const template = document.createElement('template')
			template.innerHTML = rawHtml
			const root = template.content

			const blacklistedSelectors = [
				'meta',
				'area[href]',
				'[xlink\\:href]',
			]
			const blacklistRes = root.querySelectorAll(
				blacklistedSelectors.join(','),
			)
			if (blacklistRes.length > 0) {
				return (
					<div>
						<p>Profile contains blacklisted elements.</p>
						<p>Contains at least one of:</p>
						<ul>
							<For each={blacklistedSelectors}>
								{(selector) => (
									<li>
										<code>{selector}</code>
									</li>
								)}
							</For>
						</ul>
					</div>
				)
			}

			let fsOrigin: string
			{
				const url = new URL(fsUrl)
				fsOrigin = url.origin.substring(url.protocol.length)
			}
			const uiOrigin = window.location.origin.substring(
				window.location.protocol.length,
			)

			// Check links.
			// Requirements:
			//  - No local URLs
			//  - target must be set to "_blank"
			for (const link of root.querySelectorAll('a')) {
				let linkOrigin: string
				{
					const url = new URL(link.href)
					linkOrigin = url.origin.substring(url.protocol.length)
				}

				if (linkOrigin === fsOrigin || linkOrigin === uiOrigin) {
					return (
						<div>
							<p>Profile contains invalid links.</p>
							<p>No local links allowed.</p>
							<div class={styles.code}>{link.outerHTML}</div>
						</div>
					)
				}

				if (link.target !== '_blank') {
					return (
						<div>
							<p>Profile contains invalid links.</p>
							<p>Links must have target="_blank".</p>
							<div class={styles.code}>{link.outerHTML}</div>
						</div>
					)
				}
			}

			const pathErr = (path: string, elem: Element) => {
				if (path.startsWith('/')) {
					return (
						<div>
							<p>Profile contains invalid paths.</p>
							<p>Paths must be relative.</p>
							<div class={styles.code}>{elem.outerHTML}</div>
						</div>
					)
				}

				if (path.includes('\\')) {
					return (
						<div>
							<p>Profile contains invalid paths.</p>
							<p>Paths must not contain backslashes.</p>
							<div class={styles.code}>{elem.outerHTML}</div>
						</div>
					)
				}

				const parts = path.split('/')
				for (const part of parts) {
					if (part === '..') {
						return (
							<div>
								<p>Profile contains invalid paths.</p>
								<p>Paths must not contain "..".</p>
								<div class={styles.code}>{elem.outerHTML}</div>
							</div>
						)
					}
				}

				// Path is ok.
				return undefined
			}

			// Find paths that try to escape their root directory.
			for (const elem of root.querySelectorAll('[src], [href]')) {
				const src = elem.getAttribute('src')
				if (src) {
					const err = pathErr(src, elem)
					if (err) {
						return err
					}
				}

				const href = elem.getAttribute('href')
				if (href) {
					const err = pathErr(href, elem)
					if (err) {
						return err
					}
				}
			}

			return <iframe src={indexUrl} sandbox="" />
		} catch (err) {
			console.error('failed to resolve profile:', err)
			return (
				<i>
					Failed to resolve profile. Check browser console for
					information.
				</i>
			)
		}
	})

	return (
		<div class={styles.container}>
			<Suspense fallback={<i>Loading profile...</i>}>
				{resolved()}
			</Suspense>
		</div>
	)
}

export const ServerProfilePage: Component = () => {
	const loc = useLocation()

	return (
		<Show when={loc.pathname} keyed>
			<Page />
		</Show>
	)
}
