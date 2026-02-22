import styles from './Previewer.module.css'
import stylesCommon from '../common.module.css'

import { Component, createSignal, Match, onMount, Switch } from 'solid-js'
import { PreviewInfo } from '../state'
import { guessFileCategory, makeFileUrl } from '../util'
import { useFileServerUrl, useGlobalState } from '../ctx'

type PreviewerProps = {
	info: PreviewInfo
}

type CatProps = {
	info: PreviewInfo
	url: string
	filename: string
	dir: string
}

const CatOther: Component<CatProps> = (props) => {
	const fsUrl = useFileServerUrl()

	const dlUrl = makeFileUrl(
		fsUrl,
		props.info.serverUuid,
		props.info.username,
		props.info.path,
		true,
	)

	return (
		<div class={styles.catOther}>
			<span>No Preview Available.</span>
			<br />
			<br />
			<a href={dlUrl}>💾 Download</a>
		</div>
	)
}

const CatImage: Component<CatProps> = (props) => {
	return (
		<div class={styles.catImage}>
			<img src={props.url} alt={props.filename} />
		</div>
	)
}

const CatVideo: Component<CatProps> = (props) => {
	return (
		<div class={styles.catVideo}>
			<video autoplay={true} src={props.url} controls={true} />
		</div>
	)
}

const CatText: Component<CatProps> = (props) => {
	const [isLoading, setLoading] = createSignal(true)
	const [error, setError] = createSignal('')
	const [content, setContent] = createSignal('')

	onMount(async () => {
		try {
			const res = await fetch(props.url)
			if (res.status !== 200) {
				let text: string | undefined
				try {
					text = await res.text()
				} catch (_) {}
				setError(`server return status code ${res.status}: ${text ?? ''}`)
				return
			}

			setContent(await res.text())
		} catch (err) {
			console.log('failed to load text from URL:', props.url, err)
			setError('Failed to get text content, see error in console')
		} finally {
			setLoading(false)
		}
	})

	return (
		<div class={styles.catText}>
			<Switch fallback={
				<div class={styles.content}>{content()}</div>
			}>
				<Match when={isLoading()}>
					<i>Loading...</i>
				</Match>
				<Match when={error()}>
					<div class={stylesCommon.errorMessage}>{error()}</div>
				</Match>
			</Switch>
		</div>
	)
}

const CatAudio: Component<CatProps> = (props) => {
	const fsUrl = useFileServerUrl()

	const coverFilenames = ['cover.jpg', 'cover.jpeg', 'cover.png']

	const [coverUrl, setCoverUrl] = createSignal<string | undefined>()

	onMount(async () => {
		// Try to find the cover image in the file's directory.
		for (const name of coverFilenames) {
			const url = makeFileUrl(
				fsUrl,
				props.info.serverUuid,
				props.info.username,
				`${props.dir}/${name}`,
				false,
			)
			try {
				const res = await fetch(url)

				if (res.status === 200) {
					setCoverUrl(url)
					break
				}
			} catch (err) {
				break
			}
		}
	})

	return (
		<div class={styles.catAudio}>
			<video
				autoplay={true}
				poster={coverUrl()}
				src={props.url}
				controls={true}
			/>
		</div>
	)
}

export const Previewer: Component<PreviewerProps> = (props) => {
	const state = useGlobalState()
	const fsUrl = useFileServerUrl()

	const info = props.info
	let dir: string
	let filename: string
	{
		const slashIdx = info.path.lastIndexOf('/')
		if (slashIdx === -1) {
			dir = '/'
			filename = info.path
		} else {
			dir = info.path.substring(0, slashIdx)
			filename = info.path.substring(slashIdx + 1)
		}
	}

	const url = makeFileUrl(
		fsUrl,
		info.serverUuid,
		info.username,
		info.path,
		false,
	)

	const cat = guessFileCategory(filename)
	const catProps: CatProps = {
		info,
		url,
		filename,
		dir,
	}

	return (
		<div class={styles.container}>
			<details class={stylesCommon.sidebarContainer} open={true}>
				<summary title={`Preview ${filename}`}>
					<button
						onClick={() => state.closePreview()}
						class={stylesCommon.action}
					>
						❌
					</button>

					<span class={styles.title}>Preview {filename}</span>
				</summary>

				<div class={styles.catContainer}>
					<Switch fallback={<CatOther {...catProps} />}>
						<Match when={cat === 'image'}>
							<CatImage {...catProps} />
						</Match>
						<Match when={cat === 'video'}>
							<CatVideo {...catProps} />
						</Match>
						<Match when={cat === 'audio'}>
							<CatAudio {...catProps} />
						</Match>
						<Match when={cat === 'text'}>
							<CatText {...catProps} />
						</Match>
					</Switch>
				</div>
			</details>
		</div>
	)
}
